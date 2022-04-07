#!/usr/bin/env python

import json
import logging
import os
import shlex
import shutil
import subprocess
import yaml

from base64 import b64encode
from copy import deepcopy
from jinja2 import Environment, FileSystemLoader, select_autoescape
from packaging.version import Version
from pathlib import Path
from typing import Generator, Iterable, List, Union

REGISTRY = os.getenv('REGISTRY', default='quay.io')
CATALOG_NAMESPACE = os.getenv('CATALOG_NAMESPACE', default='redhatgov/oc-mirror-dev')
IGNORED_EXTENSIONS = ('.swp',)
RUNTIME = os.getenv('CONTAINER_RUNTIME', default='podman')
SCRIPT_DIR = Path(os.path.dirname(os.path.abspath(__file__)))

# These are the definitions for what will be built

# The "latest" index contains most of the bundles
latest_index = {
    'foo': ['0.1.0', '0.2.0', '0.3.0', '0.3.1'],
    'bar': ['0.1.0', '0.2.0', '1.0.0'],
    'baz': ['1.0.0', '1.0.1', '1.1.0'],
}
# The "diff" index contains only a single extra bundle
diff_index = deepcopy(latest_index)
diff_index['foo'].append('0.3.2')
# And the minimal and minimal-update catalogs are defined inline here
CATALOGS = {
    'prune': {'foo': ['0.1.0', '0.1.1'], 'bar': ['0.1.0']},
    'prune-diff': {'foo': ['0.2.0'], 'bar': ['0.1.0']},
    'latest': latest_index,
    'diff': diff_index,
}

# Many of the files are built from jinja templates on the fly
jinja = Environment(
    loader=FileSystemLoader("templates"),
    autoescape=select_autoescape()
)


class ShellRuntimeException(RuntimeError):
    """Shell command returned non-zero return code.
    Attributes:
        code -- the return code from the shell command
    """

    def __init__(self, code: int = None, line: str = None):
        """Save the code with the exception."""
        self.code = code
        self.line = line


def _utf8ify(line_bytes: List[bytes] = None) -> str:
    """Decode line_bytes as utf-8 and strips excess whitespace."""
    return line_bytes.decode("utf-8").rstrip()


def shell(cmd: str = None, fail: bool = True) -> Iterable[str]:
    """Run a command in a subprocess, yielding lines of output from it.
    By default will cause a failure using the return code of the command. To
    change this behavior, pass fail=False.
    """
    logger.debug("Running: {}".format(cmd))
    # We are only using subprocess for specific calls, and anyone who can
    #   execute malicious code through these formatted strings via a modified
    #   configuration file (which isn't exposed to the internet) could have
    #   executed any other process already. This is specifically designed to
    #   be run as non-root, so the impact should be low when used correctly.
    proc = subprocess.Popen(shlex.split(cmd),  # nosec
                            stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT)

    last_line = None
    for line in map(_utf8ify, iter(proc.stdout.readline, b'')):
        last_line = line
        yield line

    ret = proc.wait()
    if fail and ret != 0:
        logger.error("Command errored: {}".format(cmd))
        raise ShellRuntimeException(ret, last_line)
    elif ret != 0:
        logger.warning("Command returned {}: {}".format(ret, cmd))


def build_and_push(context: Path = None, image: str = None, extra_args: str = None) -> str:
    """Call container runtime build from some directory for an image name. Returns the manifest hash of the built image."""
    logger.debug(f'Changing to {context} directory')
    prev_dir = os.getcwd()
    os.chdir(context)

    # Build
    base_command = f'{RUNTIME} build . --format=docker -t {image}'
    if extra_args is None:
        command = base_command
    else:
        command = base_command + f' {extra_args}'
    for line in shell(command):
        logger.debug(line)

    # Push
    for line in shell(f'{RUNTIME} push --format=docker {image}'):
        logger.debug(line)

    logger.debug(f'Returning to {prev_dir}')
    os.chdir(prev_dir)

    inspect_output = '\n'.join(list(shell(f'skopeo inspect docker://{image}')))
    return json.loads(inspect_output)['Digest']


class OperatorDataDirectory:
    """Class to optionally keep a temporary directory for operator data."""

    def __init__(self, path: Path = None, catalog_name: str = None, keep: bool = False) -> None:
        """Initialize the temp directory by recording the desired path."""
        prefix = 'operator-build-'
        if catalog_name is not None:
            prefix += f'{catalog_name}-'
        self.path = Path(tempfile.mkdtemp(prefix=prefix, dir=path))
        self.keep = keep

    def remove(self) -> bool:
        """Remove the temporary directory."""
        if self.keep:
            logger.debug(f'Keeping {self.path}')
            return False
        else:
            logger.info(f'Cleaning up {self.path}')
            shutil.rmtree(self.path)
            return True

    def __enter__(self) -> Path:
        """Context manager enter to create the directory."""
        return self.path

    def __exit__(self, type, value, traceback) -> None:
        """Context manager exit to remove the directory, if not to be kept."""
        self.remove()


class OperatorBundle:
    """Class that represents an individual operator bundle."""

    def __init__(self, path: Path = None) -> None:
        """Initialize the Bundle from a Path."""
        if path is not None:
            self.path = Path(path)
        self.import_data()
        self.built = False
        self.rendered = []
        self.related_image_hashes = {}

    def __repr__(self) -> str:
        """Simple representation to recreate the bundle."""
        return f'OperatorBundle({str(self.path)})'

    def __str__(self) -> str:
        """Return only the path to the bundle directory, as a string."""
        return str(self.name)

    def __eq__(self, other) -> bool:
        """Identifies equality with another bundle via path alone."""
        if isinstance(other, self.__class__) and self.path == other.path:
            return True
        if isinstance(other, str):
            if self.path == Path(other):
                return True
            if self.name == other:
                return True
        return False

    def __lt__(self, other) -> bool:
        """Identifies if the bundle should be sorted before another bundle based on version."""
        if self.package != other.package:
            raise ValueError(f'{self.name} cannot be compared to {other.name} by version.')
        return self.version < other.version

    def __gt__(self, other) -> bool:
        """Identifies if the bundle should be sorted after another bundle based on version."""
        if self.name != other.name:
            raise ValueError(f'{self.name} cannot be compared to {other.name} by version.')
        return self.version > other.version

    def import_data(self) -> None:
        """Imports data for bundle assets within its path."""
        self.csv_path = list(self.path.joinpath('manifests').glob('*.csv.yaml'))[0]
        with open(self.csv_path) as f:
            data = yaml.safe_load(f)
        self.csv = data
        self.name = self.csv.get('metadata', {}).get('name')
        self.annotations_path = self.path.joinpath('metadata').joinpath('annotations.yaml')
        with open(self.annotations_path) as f:
            data = yaml.safe_load(f)
        self.annotations = data.get('annotations', {})

    @staticmethod
    def image_rename(image_ref: str = None) -> str:
        """Rename an image reference to the new registry and catalog namespace."""
        if image_ref is not None:
            return image_ref.replace('REGISTRY_CATALOGNAMESPACE', f'{REGISTRY}/{CATALOG_NAMESPACE}')

    @property
    def img(self) -> str:
        """Construct the image name of the bundle."""
        return f'{REGISTRY}/{CATALOG_NAMESPACE}:{self.path.parts[-1]}'

    @property
    def img_by_hash(self) -> str:
        """Construct the image name of the bundle with the manifest hash reference."""
        if not self.built:
            raise RuntimeError(f'Unable to refer to image hashes for {self.name}, which is unbuilt')

    @property
    def related_imgs(self) -> Generator[dict, None, None]:
        """Yield the list of related images from the CSV of a bundle."""
        for img in self.csv.get('spec', {}).get('relatedImages'):
            yield {
                "name": img.get("name"),
                "image": self.image_rename(img.get('image'))
            }

    @property
    def related_imgs_by_hash(self) -> Generator[dict, None, None]:
        """Yield the list of related images from the CSV of a bundle, with references by hash."""
        for img in self.csv.get('spec', {}).get('relatedImages'):
            img_tag = self.image_rename(img.get('image'))
            manifest_hash = self.related_image_hashes.get(img_tag)
            if manifest_hash is None:
                raise RuntimeError(f'Unable to get manifest hash for unbuilt related image, {img_tag}.')
            img_ref_by_hash = ':'.join(img_tag.split(':')[:-1]) + '@' + manifest_hash

            yield {
                "name": img.get("name"),
                "image": img_ref_by_hash
            }

    @property
    def skip_range(self) -> str:
        """Returns the olm.skipRange annotation from the CSV of a bundle."""
        return self.csv.get('metadata', {}).get('annotations', {}).get('olm.skipRange')

    @property
    def skips(self) -> list:
        """Returns the skips field of the spec from the CSV of a bundle."""
        return self.csv.get('spec', {}).get('skips', [])

    @property
    def replaces(self) -> str:
        """Returns the replaces field of the spec form the CSV of a bundle."""
        return self.csv.get('spec', {}).get('replaces')

    @property
    def channels(self) -> list:
        """Returns every channel a bundle belongs to."""
        channels = self.annotations.get('operators.operatorframework.io.bundle.channels.v1')
        if isinstance(channels, str):
            return channels.split(',')
        else:
            return []

    @property
    def default_channel(self) -> str:
        """Returns the default channel for a bundle."""
        return self.annotations.get('operators.operatorframework.io.bundle.channel.default.v1')

    @property
    def package(self) -> str:
        """Returns the package name that a bundle should belong to."""
        return self.annotations.get('operators.operatorframework.io.bundle.package.v1').split('.', 2)[0]

    @property
    def version(self) -> Version:
        """Parses the version of a bundle from the CSV and returns it as a version object."""
        return Version(self.csv.get('spec', {}).get('version'))

    @property
    def crds(self) -> Generator[dict, None, None]:
        """Returns the CRDs from a bundle as a list of dictionaries."""
        for file in self.path.joinpath('manifests').glob('*.crd.yaml'):
            with open(file) as f:
                crd = yaml.safe_load(f)
            yield crd

    @property
    def gvks(self) -> Generator[dict, None, None]:
        """Returns the GroupVersionKinds of a bundle in olm.bundle.property schema, as dictionaries."""
        for gvk in self.csv.get('spec', {}).get('customresourcedefinitions', {}).get('owned', []):
            yield {'type': 'olm.gvk', 'value': {'group': gvk.get('group'), 'kind': gvk.get('kind'), 'version': gvk.get('version')}}

    @property
    def olm_package(self) -> dict:
        """Returns the package in olm.bundle.property schema, as a dictionary."""
        return {'type': 'olm.package', 'value': {'packageName': self.package, 'version': str(self.version)}}

    @property
    def dependencies(self) -> Generator[dict, None, None]:
        """Returns the dependency metadata for a bundle."""
        dependencies_yaml = self.path.joinpath('metadata').joinpath('dependencies.yaml')
        if not dependencies_yaml.is_file():
            return []
        with open(dependencies_yaml) as f:
            data = yaml.safe_load(f)

        for dependency in data.get('dependencies', []):
            yield dependency

    @property
    def requirements(self) -> Generator[dict, None, None]:
        """Returns a list of the dependencies in olm.bundle.property schema, as dictionaries."""
        for dependency in self.dependencies:
            depval = dependency.get('value', {})
            if dependency.get('type') == 'olm.package':
                value = {
                    'packageName': depval.get('packageName'),
                    'versionRange': depval.get('version')
                }
                yield {'type': 'olm.package.required', 'value': value}
            elif dependency.get('type') == 'olm.gvk':
                yield {'type': 'olm.gvk.required', 'value': depval}
            else:
                raise NotImplementedError(f'Dependency of type {dependency.get("type")} is not supported.')

    @property
    def objects(self) -> Generator[dict, None, None]:
        """Returns a list of bundle objects in olm.bundle.property schema, as dictionaries."""
        def objectify(thing: Union[dict, list]) -> dict:
            """Turns a given data structure into an olm.bundle.object."""
            logger.debug(f'Objectifying {thing}')
            return {'type': 'olm.bundle.object', 'value': {'data': b64encode(json.dumps(thing).encode()).decode()}}
        yield objectify(self.csv_refs_by_hash)
        for crd in self.crds:
            yield objectify(crd)

    @property
    def csv_refs_by_hash(self) -> dict:
        """Returns the bundle CSV with all image references for relatedimages replaced by their manifest hash references."""
        logger.debug('Rendering CSV with manifest hash references')
        csv_json = json.dumps(self.csv)
        for img in self.related_imgs:
            img_tag = self.image_rename(img.get('image'))
            manifest_hash = self.related_image_hashes.get(img_tag)
            if manifest_hash is None:
                raise RuntimeError(f'Unable to get manifest hash for unbuilt related image, {img_tag}.')
            img_ref_by_hash = ':'.join(img_tag.split(':')[:-1]) + '@' + manifest_hash
            logger.debug(f'Replacing references to {img_tag} with {img_ref_by_hash}')
            csv_json = csv_json.replace(img_tag, img_ref_by_hash)
        logger.debug(csv_json)
        return json.loads(csv_json)

    @property
    def properties(self) -> Generator[str, None, None]:
        """Returns the olm.bundle schema yaml blobs of properties for a bundle."""
        # Add the GVKs of the owned CRDs to the properties
        for gvk in self.gvks:
            yield gvk
        # Add the olm.package
        yield self.olm_package
        # Add the dependencies
        for requirement in self.requirements:
            yield requirement
        # Add the bundle objects
        for obj in self.objects:
            yield obj

    @property
    def index_entry(self) -> dict:
        """Return the index entry for the bundle as a dictionary.

        Intended to be used with yaml.dump or json.dump to render the index
        entry into a file-based catalog.
        """
        return {
            'schema': 'olm.bundle',
            'package': self.package,
            'name': self.name,
            'image': self.img,
            'csvJson': json.dumps(self.csv_refs_by_hash),
            'properties': list(self.properties),
            'relatedImages': list(self.related_imgs_by_hash)
        }

    def render(self, dest: Path = None) -> str:
        """Render the bundle into an alternate directory."""
        def translate_files(src: Path = None, dest: Path = None) -> None:
            """Translate files from src to dest recursively."""
            for file in src.iterdir():
                newfile = dest.joinpath(file.parts[-1])
                if file.is_dir():
                    logger.debug(f'Descending into {file}')
                    os.makedirs(newfile, exist_ok=True)
                    translate_files(src=file, dest=newfile)
                elif file.is_file():
                    if file.suffix not in IGNORED_EXTENSIONS or file.suffix == '':
                        logger.debug(f'Translating {file} into {newfile}')
                        with open(file) as f:
                            data = f.read()
                        with open(newfile, 'w') as f:
                            f.write(self.image_rename(data))

        logger.info(f'  Rendering bundle {self}')

        new_bundle_dir = dest.joinpath(self.path.parts[-1])
        new_bundle_dir.mkdir(exist_ok=True)

        logger.debug(f'Translating {self} into {dest}')
        translate_files(self.path, new_bundle_dir)

        logger.debug('Updating bundle references to rendered references.')
        old_path = self.path

        # This lets us read attributes from the rendered bundle configuration
        self.path = new_bundle_dir
        self.import_data()
        # Build relatedImages to get manifest hash references populated
        related_img_context = dest.parent.parent.joinpath('related_image')
        self.build_related(related_img_context)
        # Replace rendered CSV with manifest hash references
        with open(self.csv_path, 'w') as f:
            yaml.safe_dump(self.csv_refs_by_hash, f, explicit_start=True, sort_keys=False)

        # Grab the index entry using the translated sources and manifest hashes
        rendered_index_entry = self.index_entry
        # And add the location we've rendered to to our list of known rendered locations
        self.rendered.append(dest)
        # And finally ensure the bundle is usable for follow-on actions, even if the rendered version is deleted
        logger.debug('Returning bundle path references to source paths')
        self.path = old_path
        self.import_data()

        # Return the dictionary with the index entry
        return rendered_index_entry

    def build_related(self, context: Path = None) -> None:
        """Build related images."""
        logger.info('  Building and pushing related images to resolve manifest references')
        for img in self.related_imgs:
            img_tag = img.get('image')
            if self.related_image_hashes.get(img_tag) is None:
                logger.info(f'   Building and pushing {img_tag}')
                self.related_image_hashes[img_tag] = build_and_push(context, img_tag, f'--build-arg RELATED_IMAGE={img_tag}')
                logger.debug(f'Digest: {self.related_image_hashes[img_tag]}')
            else:
                logger.info(f' Already built {img_tag} at {self.related_image_hashes[img_tag]}')

    def build_and_push(self, context: Path = None) -> None:
        """Build bundle."""
        logger.info(f' Building and pushing {self.img}')
        self.bundle_digest = build_and_push(context, self.img)
        logger.debug(f'Digest: {self.bundle_digest}')

        self.built = True


class OperatorChannel:
    """Class that represents a channel of bundles in an operator package."""

    def __init__(self, name: str = None, bundles: list = []) -> None:
        """Initializes a channel of bundles in a package."""
        self.name = name
        self.bundles = bundles

    def __repr__(self) -> str:
        """Simple representation to recreate the channel."""
        return f'OperatorChannel({self.name}, {self.bundles})'

    def __str__(self) -> str:
        """Return only the name of the channel."""
        return self.name

    def __eq__(self, other) -> bool:
        """Identifies equality with another channel via name."""
        if isinstance(other, self.__class__) and self.name == other.name:
            return True
        if isinstance(other, str) and self.name == other:
            return True
        return False

    def __iter__(self) -> Generator[OperatorBundle, None, None]:
        """Iterate over bundles in the channel."""
        yield from self.bundles

    def add_bundle(self, bundle: OperatorBundle = None) -> None:
        """Adds a bundle to the channel."""
        if bundle not in self.bundles:
            logger.debug(f'  Adding {bundle} to {self.name}')
            self.bundles.append(bundle)
            self.bundles.sort()

    @property
    def package(self) -> str:
        """Return the package of the first bundle, assuming it is correct."""
        return self.bundles[0].package

    @property
    def index_entry(self) -> dict:
        """Return the index entry for the channel as a dictionary.

        Intended to be used with yaml.dump or json.dump to render the index
        entry into a file-based catalog.
        """
        entries = []
        for bundle in self:
            entry = {'name': bundle.name}
            if bundle.skip_range is not None:
                entry['skipRange'] = bundle.skip_range
            if bundle.skips:
                entry['skips'] = list(bundle.skips)
            if bundle.replaces is not None and bundle.replaces in self.bundles:
                entry['replaces'] = bundle.replaces
            entries.append(entry)
        return {
            'schema': 'olm.channel',
            'package': self.package,
            'name': self.name,
            'entries': entries
        }


class OperatorPackage:
    """Class that represents a package for an operator in an index."""

    def __init__(self, name: str = None, versions: list = [], bundles_from: Path = None) -> None:
        """Initializesa package for an operator in an index."""
        self.name = name
        self.channels = []
        self.default_channel = None
        self.versions = versions
        self.bundles_from = bundles_from
        for version in versions:
            bundle_dir = bundles_from.joinpath(name).joinpath(f'{name}-bundle-v{version}')
            if bundle_dir.is_dir():
                bundle = OperatorBundle(bundle_dir)
                for channel_name in bundle.channels:
                    self.add_channel(OperatorChannel(channel_name, []))
                    self.channel(channel_name).add_bundle(bundle)
                self.default_channel = bundle.default_channel
            else:
                logger.warning(f'{bundle_dir} does not appear to be present for {name}')

    def __repr__(self) -> str:
        """Simple representation to recreate the package."""
        return f'OperatorPackage({self.name}, {self.versions},  {self.bundles_from})'

    def __str__(self) -> str:
        """Return only the name of the package."""
        return self.name

    def __eq__(self, other) -> bool:
        """Identify equality with another package via name."""
        if isinstance(other, self.__class__) and self.name == other.name:
            return True
        if isinstance(other, str) and self.name == other:
            return True
        return False

    def __iter__(self) -> Generator[OperatorChannel, None, None]:
        """Iterate over channels in the package."""
        yield from self.channels

    @property
    def dir(self) -> Path:
        """Return the (assumed) directory of the operator package."""
        return Path(os.getcwd()).joinpath('bundles').joinpath(self.name)

    @property
    def description(self) -> str:
        """Read the description from README.md."""
        with open(self.dir.joinpath('README.md')) as f:
            data = f.read()
        return data

    @property
    def icon(self) -> str:
        """Read the icon from disk and return it as a base64-encoded value."""
        with open(self.dir.joinpath(f'{self.name}.svg')) as f:
            data = f.read()
        return b64encode(data.encode()).decode()

    def add_channel(self, channel: OperatorChannel = None) -> None:
        """Adds a channel of bundles to the package."""
        if channel not in self.channels:
            logger.debug(f'  Adding {channel} to {self.name}')
            self.channels.append(channel)

    def channel(self, channel_name: str = None) -> OperatorChannel:
        """Find a channel from the list of channels by name."""
        for channel in self.channels:
            if channel == channel_name:
                return channel

    @property
    def index_entry(self) -> dict:
        """Return the index entry for the package as a dictionary.

        Intended to be used with yaml.dump or json.dump to render the index
        entry into a file-based catalog.
        """
        return {
            'schema': 'olm.package',
            'name': self.name,
            'description': self.description,
            'icon': {
                'base64data': self.icon,
                'mediatype': 'image/svg+xml'
            },
            'defaultChannel': self.default_channel
        }

    def render(self, src: Path = None, dest: Path = None, index: Path = None) -> None:
        """Render all bundles in a package into a new directory."""
        # rendered_package is the index.yaml content for this package in the index
        rendered = [self.index_entry]

        # This is used to prevent duplication
        logger.info(f'  Rendering package {self}')
        for channel in self:
            # Add channel data based on bundles in the channel
            rendered.append(channel.index_entry)
            # Render bundle data as well
            for bundle in channel:
                # Only if it hasn't been rendered yet
                if dest not in bundle.rendered:
                    rendered.append(bundle.render(dest=dest))

        # Write the package index.yaml
        with open(index, 'a') as f:
            yaml.dump_all(rendered, f, explicit_start=True, sort_keys=False)

        # And copy over the package image and description
        for package_file in (f'{self.name}.svg', 'README.md'):
            copy_from = src.joinpath(package_file)
            copy_to = dest.joinpath(package_file)
            logger.debug(f'Copying {copy_from} to {copy_to}.')
            with open(copy_from) as f:
                data = f.read()
            with open(copy_to, 'w') as f:
                f.write(data)


class OperatorIndex:
    """Class that represents an index of packages."""

    def __init__(self, name: str = None, packages: dict = {}, bundles_from: Path = None, single_yaml: bool = True) -> None:
        """Initializes an index of packages."""
        self.name = name
        self._create_packages = packages
        self.bundles_from = bundles_from
        self.single_yaml = single_yaml
        # Read through the packages dictionary to create OperatorPackages within the index context
        self.packages = []
        for package in packages:
            self.packages.append(OperatorPackage(package, versions=packages[package], bundles_from=bundles_from))
        # These are set to true when the methods have been called
        self.rendered = False
        self.built = False
        # The path will be populated when the catalog is rendered.
        self.path = None

    def __repr__(self) -> str:
        """Simple representation to recreate the index."""
        return f'OperatorIndex({self.name}, {self._create_packages}, {self.bundles_from})'

    def __str__(self) -> str:
        """Return only the name of the index."""
        return self.name

    def __iter__(self) -> Generator[OperatorPackage, None, None]:
        """Iterate over the packages in the index."""
        yield from self.packages

    def add_package(self, package: OperatorPackage = None) -> None:
        """Adds a package to the index."""
        if package not in self.packages:
            logger.debug(f'  Adding {package} to {self.name}')
            self.packages.append(package)

    def package(self, package_name: str = None) -> OperatorPackage:
        """Finds a package from the list of packages by name."""
        for package in self.packages:
            if package == package_name:
                return package

    @property
    def img(self) -> str:
        """Construct the image name of the catalog index."""
        return f'{REGISTRY}/{CATALOG_NAMESPACE}:{self.name}'

    def render(self, src: Path = None, dest: Path = None) -> None:
        """Render all the packages and the index into dest."""
        logger.info(f'Rendering {self.name} catalog indexes.')
        bundle_dir = src.joinpath('bundles')
        new_bundle_dir = dest.joinpath('bundles')
        new_bundle_dir.mkdir(exist_ok=True)

        new_catalog_dir = dest.joinpath('catalog')
        # Save the path for future catalog operations
        self.path = new_catalog_dir
        new_catalog_dir.mkdir(exist_ok=True)

        index_dir = new_catalog_dir.joinpath(self.name)
        index_dir.mkdir(exist_ok=True)

        # This is only used if single_yaml is true
        single_index_yaml = index_dir.joinpath('index.yaml')

        # The related_image can be copied wholesale, as we use a build arg
        related_img_dir = src.joinpath('related_image')
        new_related_img_dir = dest.joinpath('related_image')
        shutil.copytree(related_img_dir, new_related_img_dir)

        # Render the packages
        for package in self:
            logger.info(f' Processing {package.name} index')
            package_dir = bundle_dir.joinpath(package.name)
            new_package_dir = new_bundle_dir.joinpath(package.name)
            new_package_dir.mkdir(exist_ok=True)

            if self.single_yaml:
                package.render(src=package_dir, dest=new_package_dir, index=single_index_yaml)
            else:
                package_index_dir = index_dir.joinpath(package.name)
                package_index_dir.mkdir(exist_ok=True)
                package_index_yaml = package_index_dir.joinpath('index.yaml')

                package.render(src=package_dir, dest=new_package_dir, index=package_index_yaml)

        # Render the catalog source and dockerfile
        for file in ('Dockerfile', 'catalogSource.yaml'):
            logger.debug(f'Rendering {file} into {new_catalog_dir}')
            newfile = new_catalog_dir.joinpath(file)
            template = jinja.get_template(file)
            rendered = template.render(index=self)
            with open(newfile, 'w') as f:
                f.write(rendered)

        self.rendered = True

    def build_and_push(self, context: Path = None, catalog_only: bool = False) -> None:
        """Build all associated container images using the context directory."""
        logger.info(f'Building images for {self.name} catalog.')

        catalog_context = context.joinpath('catalog')
        bundle_root_context = context.joinpath('bundles')

        logger.info(f' Building and pushing {self.img}')
        build_and_push(catalog_context, self.img)

        if catalog_only:
            self.built = True
            return None

        for package in self:
            bundle_package_context = bundle_root_context.joinpath(package.name)
            for channel in package:
                for bundle in channel:
                    bundle_context = bundle_package_context.joinpath(bundle.path.parts[-1])
                    if not bundle.built:
                        bundle.build_and_push(bundle_context)
                    else:
                        logger.debug(f'{bundle.img} appears to already be built.')

        self.built = True

    @property
    def catalog_source_path(self) -> Path:
        """The path that a rendered catalog_source would be at."""
        return self.path.joinpath('catalogSource.yaml')

    @property
    def catalog_source(self) -> str:
        """Rendered catalog_source for an index."""
        if not self.rendered:
            raise RuntimeError('Unable to render a CatalogSource for an unrendered Index.')
        if not self.built:
            logger.warning('CatalogSource will be invalid without published images, and they have not yet been published.')
        with open(self.catalog_source_path) as f:
            data = f.read()
        return data


if __name__ == '__main__':
    import argparse
    import tempfile

    parser = argparse.ArgumentParser(description='Analyze CSVs and prepare a catalog index, publishing all images.',
                                     epilog='Note that Skopeo is required to be installed, regardless of runtime used for building and pushing.')
    parser.add_argument('-a', '--apply', help='apply a catalog to an OpenShift cluster using the oc cli', default=None)
    parser.add_argument('-c', '--catalogs', help='a comma-separated list of catalogs to build', default='latest,diff,prune,prune-diff')
    parser.add_argument('-k', '--keep', help='keep the temporary working directory', action='store_true')
    parser.add_argument('-l', '--log', help='a log file to set for debug logging')
    parser.add_argument('-n', '--no-build', help="don't build and publish the images, simply render them", action='store_true')
    parser.add_argument('-r', '--runtime', help='the container runtime to use for building and pushing', default=RUNTIME)
    parser.add_argument('-s', '--split', help='also build split catalogs, with folders and indexes per package', action='store_true')
    parser.add_argument('-v', '--verbose', help='increase output verbosity', action='store_true')
    args = parser.parse_args()

    RUNTIME = args.runtime

    logger = logging.getLogger('publish_images')
    logger.setLevel(logging.DEBUG)
    _format = '{asctime} {name} [{levelname:^9s}]: {message}'
    formatter = logging.Formatter(_format, style='{')
    stderr = logging.StreamHandler()
    stderr.setFormatter(formatter)
    if args.verbose:
        stderr.setLevel(logging.DEBUG)
    else:
        stderr.setLevel(logging.INFO)
    logger.addHandler(stderr)
    if args.log is not None:
        logfile = logging.FileHandler(filename=args.log, mode='w')
        logfile.setFormatter(formatter)
        logfile.setLevel(logging.DEBUG)
        logger.addHandler(logfile)

    # Script is expected to be run adjacent to the bundles
    operator_dir = SCRIPT_DIR

    # Used to help identify which catalog to apply
    apply_path = None

    # Mark which bundles we're using to pass to the index
    bundle_source_dir = operator_dir.joinpath('bundles')
    for catalog in args.catalogs.split(','):
        # Allow the index to parse itself
        index = OperatorIndex(f'test-catalog-{catalog}', packages=CATALOGS[catalog], bundles_from=bundle_source_dir, single_yaml=True)
        # Render and build all images for the index
        with OperatorDataDirectory(path=SCRIPT_DIR, catalog_name=catalog, keep=args.keep) as data_dir:
            index.render(src=operator_dir, dest=data_dir)
            if not args.no_build:
                index.build_and_push(context=data_dir, catalog_only=False)
            print(index.catalog_source)
            if args.apply is not None and catalog == args.apply:
                logger.info(f'Applying {index} to cluster')
                try:
                    for line in shell(f'oc apply -f "{index.catalog_source_path}"'):
                        logger.debug(line)
                except ShellRuntimeException as e:
                    logger.error(f'Unable to apply {index} catalogSource to cluster: {e}')
            else:
                logger.debug(f'Not applying {index}')

        if args.split:
            # Generate an exact copy of the index, with split package indexes
            index.name = f'test-catalog-{catalog}-split'
            index.rendered = False
            index.built = False
            index.single_yaml = False
            with OperatorDataDirectory(path=SCRIPT_DIR, catalog_name=f'{catalog}-split', keep=args.keep) as data_dir:
                index.render(src=operator_dir, dest=data_dir)
                if not args.no_build:
                    index.build_and_push(context=data_dir, catalog_only=True)
                print(index.catalog_source)
