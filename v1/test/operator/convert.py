#!/usr/bin/env python

# This script should be run only if you need to update all of the bundles
# It was used after editing foo-bundle-v0.1.0/manifests/foo.csv.yaml in 9b74c989471f146296d55c49faa10eba9feebfb1
# It was specifically designed to propogate those changes to all other bundles in a8389232dcface7555afdf8bbd2c95dd0bc9f95d
# You may need to change what gets popped off of the source template at some point if you need to update other bundle data en masse

import os
import yaml
import logging
from base64 import b64encode
from pathlib import Path
from typing import Any

logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.DEBUG)

operators = ('foo', 'bar', 'baz')
SCRIPT_DIR = Path(os.path.dirname(os.path.abspath(__file__)))
bundle_dir = SCRIPT_DIR.joinpath('bundles')
template_bundle = bundle_dir.joinpath('foo').joinpath('foo-bundle-v0.1.0')
template_csv = template_bundle.joinpath('manifests').joinpath('foo.csv.yaml')
with open(template_csv) as f:
    template_data = yaml.safe_load(f)
template_data['metadata'].pop('name')
template_data['spec'].pop('customresourcedefinitions')
template_data['spec'].pop('version')
try:
    template_data['spec'].pop('replaces')
except KeyError:
    pass
try:
    template_data['spec'].pop('skips')
except KeyError:
    pass
template_data['spec'].pop('relatedImages')
template_data['spec'].pop('icon')
logging.info(f'Template: {template_data}')


def merge(a, b, path=None):
    "merges b into a"
    if path is None:
        path = []
    for key in b:
        if key in a:
            if isinstance(a[key], dict) and isinstance(b[key], dict):
                merge(a[key], b[key], path + [str(key)])
            elif a[key] == b[key]:
                pass  # same leaf value
            else:
                raise Exception('Conflict at %s' % '.'.join(path + [str(key)]))
        else:
            a[key] = b[key]
    return a


def convert_bundle(src: Path) -> str:
    def convert_refs(src: Any) -> Any:
        if isinstance(src, dict):
            ret = {}
            for k, v in src.items():
                ret[k] = convert_refs(v)
        elif isinstance(src, list):
            ret = []
            for v in src:
                ret.append(convert_refs(v))
        elif isinstance(src, str):
            return src.replace('0.1.0', version).replace('foo', name).replace('Foo', name.title())
        elif isinstance(src, bool) or isinstance(src, int):
            return src
        else:
            raise NotImplementedError(f'no case for object of type {type(src)}')
        return ret

    csv_path = list(src.joinpath('manifests').glob('*.csv.yaml'))[0]
    with open(csv_path) as f:
        bundle_data = yaml.safe_load(f)
    version = bundle_data['spec']['version']
    name = bundle_data['metadata']['name'].split('.')[0]

    if src == template_bundle:
        new_bundle_data = bundle_data
    else:
        # Temporarily inherit these from the template
        bundle_data['spec'].pop('installModes')
        new_bundle_data = merge(bundle_data, convert_refs(template_data))
    icon_path = src.parent.joinpath(f'{name}.svg')
    with open(icon_path) as f:
        icon_data = f.read()
    b64_icon_data = b64encode(icon_data.encode()).decode()
    new_bundle_data['spec']['icon'][0]['base64data'] = b64_icon_data

    with open(csv_path, 'w') as f:
        f.write(yaml.dump(new_bundle_data, explicit_start=True))


for operator in operators:
    operator_dir = bundle_dir.joinpath(operator)
    for bundle in operator_dir.glob(f'{operator}-bundle*'):
        convert_bundle(bundle)
