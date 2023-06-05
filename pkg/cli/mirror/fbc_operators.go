package mirror

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	imagecopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/cli/environment"
	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/opencontainers/go-digest"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

const (
	blobsPath           string = "/blobs/sha256/"
	dockerPrefix        string = "docker://"
	configPath          string = "configs/"
	catalogJSON         string = "/catalog.json"
	relatedImages       string = "relatedImages"
	artifactsFolderName string = "olm_artifacts"
	ocpRelease          string = "release"
	ocpReleaseImages    string = "release-images"
	filePrefix          string = "file://"
	sha256Tag           string = "sha256:"
	manifests           string = "manifests"
	openshift           string = "openshift"
	source              string = "src/v2"
)

// RemoteRegFuncs contains the functions to be used for working with remote registries
// In order to be able to mock these external packages,
// we pass them as parameters of bulkImageCopy and bulkImageMirror
type RemoteRegFuncs struct {
	copy               func(ctx context.Context, policyContext *signature.PolicyContext, destRef types.ImageReference, srcRef types.ImageReference, options *imagecopy.Options) (copiedManifest []byte, retErr error)
	mirrorMappings     func(cfg v1alpha2.ImageSetConfiguration, images image.TypedImageMapping, insecure bool) error
	newImageSource     func(ctx context.Context, sys *types.SystemContext, imgRef types.ImageReference) (types.ImageSource, error)
	getManifest        func(ctx context.Context, instanceDigest *digest.Digest, imgSrc types.ImageSource) ([]byte, string, error)
	handleMetadata     func(ctx context.Context, tmpdir string, filesInArchive map[string]string) (backend storage.Backend, incoming, curr v1alpha2.Metadata, err error)
	m2mWorkflowWrapper func(ctx context.Context, cfg v1alpha2.ImageSetConfiguration, cleanup cleanupFunc) error
}

// getISConfig simple function to read and unmarshal the imagesetconfig
// set via the command line
func (o *MirrorOptions) getISConfig() (*v1alpha2.ImageSetConfiguration, error) {
	var isc *v1alpha2.ImageSetConfiguration
	configData, err := os.ReadFile(o.ConfigPath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(configData, &isc)
	if err != nil {
		return nil, err
	}
	return isc, nil
}

func (o *MirrorOptions) generateSrcToFileMapping(ctx context.Context, relatedImages []declcfg.RelatedImage) (image.TypedImageMapping, error) {
	mapping := image.TypedImageMapping{}
	for _, i := range relatedImages {
		if i.Image == "" {
			klog.Warningf("invalid related image %s: reference empty", i.Name)
			continue
		}
		reg, err := sysregistriesv2.FindRegistry(newSystemContext(o.SourceSkipTLS, o.OCIRegistriesConfig), i.Image)
		if err != nil {
			klog.Warningf("Cannot find registry for %s: %v", i.Image, err)
		}
		if reg != nil && len(reg.Mirrors) > 0 {
			// i.Image is coming from a declarativeConfig (ClusterServiceVersion) it's therefore always a docker ref
			mirroredImage, err := findFirstAvailableMirror(ctx, reg.Mirrors, dockerPrefix+i.Image, reg.Prefix, o.remoteRegFuncs)
			if err == nil {
				i.Image = mirroredImage
			}
		}

		srcTIR, err := image.ParseReference(i.Image)
		if err != nil {
			return nil, err
		}
		srcTI := image.TypedImage{
			TypedImageReference: srcTIR,
			Category:            v1alpha2.TypeOperatorRelatedImage,
		}
		// The registry is needed in from, as this will be used to generate ICSP from mapping
		dstPath := filePrefix + srcTIR.Ref.Registry + "/" + srcTIR.Ref.Namespace + "/" + srcTIR.Ref.Name
		if srcTIR.Ref.ID != "" {
			dstPath = dstPath + "/" + strings.TrimPrefix(srcTI.Ref.ID, sha256Tag)
		} else if srcTIR.Ref.ID == "" && srcTIR.Ref.Tag != "" {
			//recreating a fake digest to copy image into
			//this is because dclcfg.LoadFS will create a symlink to this folder
			//from the tag
			dstPath = dstPath + "/" + fmt.Sprintf("%x", sha256.Sum256([]byte(srcTIR.Ref.Tag)))[0:6]
		}
		dstTIR, err := image.ParseReference(strings.ToLower(dstPath))
		if err != nil {
			return nil, err
		}
		if srcTI.Ref.Tag != "" {
			//put the tag back because it's needed to follow symlinks by LoadFS
			dstTIR.Ref.Tag = srcTI.Ref.Tag
		}
		dstTI := image.TypedImage{
			TypedImageReference: dstTIR,
			Category:            v1alpha2.TypeOperatorRelatedImage,
		}
		mapping[srcTI] = dstTI
	}
	return mapping, nil
}

func (o *MirrorOptions) addRelatedImageToMapping(ctx context.Context, mapping *sync.Map, img declcfg.RelatedImage, destReg, namespace string) error {
	if img.Image == "" {
		klog.Warningf("invalid related image %s: reference empty", img.Name)
		return nil
	}
	reg, err := sysregistriesv2.FindRegistry(newSystemContext(o.SourceSkipTLS, o.OCIRegistriesConfig), img.Image)
	if err != nil {
		klog.Warningf("Cannot find registry for %s", img.Image)
	}

	from, to := "", ""
	_, subns, imgName, tag, sha := v1alpha2.ParseImageReference(img.Image)
	if imgName == "" {
		return fmt.Errorf("invalid related image %s: repository name empty", img.Image)
	}

	from = img.Image

	if reg != nil && len(reg.Mirrors) > 0 {
		// i.Image is coming from a declarativeConfig (ClusterServiceVersion) it's therefore always a docker ref
		mirroredImage, err := findFirstAvailableMirror(ctx, reg.Mirrors, dockerPrefix+img.Image, reg.Prefix, o.remoteRegFuncs)
		if err == nil {
			from = mirroredImage
		} else {
			// verbose log so we know when we had no mirror hits
			klog.V(3).Infof("Cannot find mirror for %s: %s", img.Image, err)
		}
	}

	to = destReg
	if namespace != "" {
		to = strings.Join([]string{to, namespace}, "/")
	}
	if subns != "" {
		to = strings.Join([]string{to, subns}, "/")
	}
	to = strings.Join([]string{to, imgName}, "/")
	if tag != "" {
		to = to + ":" + tag
	} else {
		to = to + "@" + sha256Tag + sha
	}

	// TODO : why file:// ? can we be more smart about setting the transport protocol
	srcTIR, err := image.ParseReference(strings.ToLower(from))
	if err != nil {
		return err
	}
	if sha != "" && srcTIR.Ref.ID == "" {
		srcTIR.Ref.ID = sha256Tag + sha
	}
	if tag != "" && srcTIR.Ref.Tag == "" {
		srcTIR.Ref.Tag = tag
	}
	srcTI := image.TypedImage{
		TypedImageReference: srcTIR,
		Category:            v1alpha2.TypeOperatorRelatedImage,
	}

	dstTIR, err := image.ParseReference(to)
	if err != nil {
		return err
	}
	if sha != "" && dstTIR.Ref.ID == "" {
		dstTIR.Ref.ID = sha256Tag + sha
	}
	//If there is no tag mirrorMapping is unable to push the image
	//It would push manifests and layers, but image would not appear
	//in registry
	if sha != "" && dstTIR.Ref.Tag == "" {
		dstTIR.Ref.Tag = sha[0:6]
	}
	dstTI := image.TypedImage{
		TypedImageReference: dstTIR,
		Category:            v1alpha2.TypeOperatorRelatedImage,
	}
	mapping.Store(srcTI, dstTI)
	return nil
}

func prepareDestCatalogRef(operator v1alpha2.Operator, destReg, namespace string) (string, error) {
	if destReg == "" {
		return "", errors.New("destination registry may not be empty")
	}
	uniqueName, err := operator.GetUniqueName()
	if err != nil {
		return "", err
	}
	notAReg, subNamespace, repo, tag, _ := v1alpha2.ParseImageReference(uniqueName)

	to := dockerPrefix + destReg

	if namespace != "" {
		to = strings.Join([]string{to, namespace}, "/")
	}
	if notAReg != "" {
		to = strings.Join([]string{to, notAReg}, "/")
	}
	if subNamespace != "" {
		to = strings.Join([]string{to, subNamespace}, "/")
	}

	to = strings.Join([]string{to, repo}, "/")
	if tag != "" {
		to += ":" + tag
	}
	//check if this is a valid reference
	_, err = image.ParseReference(v1alpha2.TrimProtocol(to))
	return to, err
}

func addCatalogToMapping(catalogMapping image.TypedImageMapping, srcOperator v1alpha2.Operator, digest digest.Digest, destRef string) error {
	if digest == "" {
		return fmt.Errorf("no digest provided for OCI catalog %s after copying it to the disconnected registry. This usually indicates an error in the catalog copy", srcOperator.Catalog)
	}
	// need to use GetUniqueName, because JUST for the catalogSource
	// generation, we need the srcOperator reference to be based on
	// targetName and targetTag if they exist
	srcCtlgRef, err := srcOperator.GetUniqueName()
	if err != nil {
		return err
	}
	if srcOperator.IsFBCOCI() && !strings.Contains(srcCtlgRef, v1alpha2.OCITransportPrefix) {
		srcCtlgRef = v1alpha2.OCITransportPrefix + "//" + srcCtlgRef
	}

	ctlgSrcTIR, err := image.ParseReference(srcCtlgRef)
	if err != nil {
		return err
	}

	ctlgDstTIR, err := image.ParseReference(v1alpha2.TrimProtocol(destRef))
	if err != nil {
		return err
	}
	// digest is returned from the result of copy to the disconnected registry, and unless there is
	// an error during copy, this digest will be provided.
	// ctlgSrcTIR.Ref.ID will not be empty for the case of a remote registry, but for oci FBC catalogs,
	// this will always be empty.
	// if both digest and ctlgSrcTIR.Ref.ID are empty, then there is no way of creating a accurate mapping source.
	if digest == "" && ctlgSrcTIR.Ref.ID == "" {
		return fmt.Errorf("unable to add catalog %s to mirror mapping: no digest found", srcOperator.Catalog)
	}
	if digest != "" && ctlgSrcTIR.Ref.ID == "" {
		ctlgSrcTIR.Ref.ID = string(digest)
	}
	if ctlgSrcTIR.Ref.ID != "" && ctlgDstTIR.Ref.ID == "" {
		ctlgDstTIR.Ref.ID = ctlgSrcTIR.Ref.ID
	}
	if ctlgSrcTIR.Ref.Tag != "" && ctlgDstTIR.Ref.Tag == "" {
		ctlgDstTIR.Ref.Tag = ctlgSrcTIR.Ref.Tag
	}

	ctlgSrcTI := image.TypedImage{
		TypedImageReference: ctlgSrcTIR,
		Category:            v1alpha2.TypeOperatorCatalog,
	}

	ctlgDstTI := image.TypedImage{
		TypedImageReference: ctlgDstTIR,
		Category:            v1alpha2.TypeOperatorCatalog,
	}

	if srcOperator.IsFBCOCI() {
		ctlgSrcTI.ImageFormat = image.OCIFormat
		ctlgDstTI.ImageFormat = image.OCIFormat
	}

	catalogMapping[ctlgSrcTI] = ctlgDstTI
	return nil
}

/*
extractDeclarativeConfigFromImage obtains a DeclarativeConfig instance from an image

# Arguments

• img: the image to pull a DeclarativeConfig out of

• extractedImageDir: the location where the DeclarativeConfig should be placed upon extraction.
Typically <current working directory>/olm_artifacts/<repo>.

# Returns

• string: path to the folder containing the DeclarativeConfig if no error occurred, otherwise empty string.
The config directory from img is determined and appended to extractedImageDir.
Typically results in <current working directory>/olm_artifacts/<repo>/<config folder>

• error: non-nil if an error occurred, nil otherwise
*/
func extractDeclarativeConfigFromImage(img v1.Image, extractedImageDir string) (string, error) {
	if img == nil {
		return "", errors.New("unable to extract DeclarativeConfig because no image was provided")
	}

	config, err := img.ConfigFile()
	if err != nil {
		return "", err
	}
	configsPrefix := "configs/"
	if config.Config.Labels != nil {
		label := config.Config.Labels[containertools.ConfigsLocationLabel]
		if label != "" {
			// strip beginning slash since this would prevent the configsPrefix from matching later on
			label = strings.TrimPrefix(label, "/")
			// since configsPrefix is supposed to be a directory, put in the ending slash if its not already present.
			if !strings.HasSuffix(label, "/") {
				label = label + "/"
			}
			configsPrefix = label
		}
	}
	returnPath := filepath.Join(extractedImageDir, configsPrefix)

	tr := tar.NewReader(mutate.Extract(img))
	for {
		header, err := tr.Next()

		// break the infinite loop when EOF
		if errors.Is(err, io.EOF) {
			break
		}

		// skip the file if it is a directory or not in the configs dir
		if !strings.HasPrefix(header.Name, configsPrefix) || header.FileInfo().IsDir() {
			continue
		}

		var buf bytes.Buffer
		_, err = buf.ReadFrom(tr)
		if err != nil {
			return "", err
		}

		targetFileName := filepath.Join(extractedImageDir, header.Name)
		bytes := buf.Bytes()

		baseDir := filepath.Dir(targetFileName)
		err = os.MkdirAll(baseDir, 0755)
		if err != nil {
			return "", err
		}

		f, err := os.Create(targetFileName)
		if err == nil {
			defer f.Close()
		} else {
			return "", err
		}

		_, err = f.Write(bytes)
		if err != nil {
			return "", err
		}
	}
	// check for the folder (it should exist if we found something)
	_, err = os.Stat(returnPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("directory not found after extracting %q within image", configsPrefix)
	}
	// folder itself should contain data
	dirs, err := os.ReadDir(returnPath)
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("no content found at %q within image", configsPrefix)
	}
	return returnPath, nil
}

// getRelatedImages reads a directory containing an FBC catalog () unpacked contents
// and returns the list of relatedImages found in the CSVs of bundles
// filtering by the list of packages provided in imageSetConfig for the catalog
func getRelatedImages(cfg declcfg.DeclarativeConfig) ([]declcfg.RelatedImage, error) {
	allImages := []declcfg.RelatedImage{}

	for _, bundle := range cfg.Bundles {
		allImages = append(allImages, declcfg.RelatedImage{Name: bundle.Package, Image: bundle.Image})
		allImages = append(allImages, bundle.RelatedImages...)
	}
	//make sure there are no duplicates in the list with same image:
	finalList := []declcfg.RelatedImage{}
	for _, i := range allImages {
		found := false
		for _, j := range finalList {
			if i.Image == j.Image {
				found = true
				break
			}
		}
		if !found {
			finalList = append(finalList, i)
		}
	}
	return finalList, nil
}

func findFirstAvailableMirror(ctx context.Context, mirrors []sysregistriesv2.Endpoint, imageName string, prefix string, regFuncs RemoteRegFuncs) (string, error) {
	finalError := fmt.Errorf("could not find a valid mirror for %s", imageName)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	for _, mirror := range mirrors {
		if !strings.HasSuffix(mirror.Location, "/") {
			mirror.Location += "/"
		}
		mirroredImage := strings.Replace(imageName, prefix, mirror.Location, 1)
		imgRef, err := alltransports.ParseImageName(mirroredImage)
		if err != nil {
			finalError = fmt.Errorf("%w: unable to parse reference %s: %v", finalError, mirroredImage, err)
			continue
		}
		imgsrc, err := regFuncs.newImageSource(ctx, nil, imgRef)
		defer func() {
			if imgsrc != nil {
				err = imgsrc.Close()
				if err != nil {
					klog.V(3).Infof("%s is not closed", imgsrc)
				}
			}
		}()
		if err != nil {
			finalError = fmt.Errorf("%w: unable to create ImageSource for %s: %v", finalError, mirroredImage, err)
			continue
		}
		_, _, err = regFuncs.getManifest(ctx, nil, imgsrc)
		if err != nil {
			finalError = fmt.Errorf("%w: unable to get Manifest for %s: %v", finalError, mirroredImage, err)
			continue
		} else {
			return v1alpha2.TrimProtocol(mirroredImage), nil
		}
	}
	return "", finalError
}

// copyImage is used both for pulling catalog images from the remote registry
// as well as pushing these catalog images to the remote registry.
// It calls the underlying containers/image copy library, which looks out for registries.conf
// file if any, when copying images around.
func (o *MirrorOptions) copyImage(ctx context.Context, from, to string, funcs RemoteRegFuncs) (digest.Digest, error) {
	if !strings.HasPrefix(from, "docker") {
		// find absolute path if from is a relative path
		fromPath := v1alpha2.TrimProtocol(from)
		if !strings.HasPrefix(fromPath, "/") {
			absolutePath, err := filepath.Abs(fromPath)
			if err != nil {
				return digest.Digest(""), fmt.Errorf("unable to get absolute path of oci image %s: %v", from, err)
			}
			from = "oci://" + absolutePath
		}
	}

	sourceCtx := newSystemContext(o.SourceSkipTLS, o.OCIRegistriesConfig)
	destinationCtx := newSystemContext(o.DestSkipTLS, "")

	// Pull the source image, and store it in the local storage, under the name main
	var sigPolicy *signature.Policy
	var err error
	if o.OCIInsecureSignaturePolicy {
		sigPolicy = &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	} else {
		sigPolicy, err = signature.DefaultPolicy(nil)
		if err != nil {
			return digest.Digest(""), err
		}
	}
	policyContext, err := signature.NewPolicyContext(sigPolicy)
	if err != nil {
		return digest.Digest(""), err
	}
	// define the source context
	srcRef, err := alltransports.ParseImageName(from)
	if err != nil {
		return digest.Digest(""), err
	}
	// define the destination context
	destRef, err := alltransports.ParseImageName(to)
	if err != nil {
		return digest.Digest(""), err
	}

	// call the copy.Image function with the set options
	manifestBytes, err := funcs.copy(ctx, policyContext, destRef, srcRef, &imagecopy.Options{
		RemoveSignatures:      true,
		SignBy:                "",
		ReportWriter:          os.Stdout,
		SourceCtx:             sourceCtx,
		DestinationCtx:        destinationCtx,
		ForceManifestMIMEType: "",
		ImageListSelection:    imagecopy.CopySystemImage,
		OciDecryptConfig:      nil,
		OciEncryptLayers:      nil,
		OciEncryptConfig:      nil,
	})
	if err != nil {
		return digest.Digest(""), err
	}
	return manifest.Digest(manifestBytes)
}

// newSystemContext set the context for source & destination resources
func newSystemContext(skipTLS bool, registriesConfigPath string) *types.SystemContext {
	skipTLSVerify := types.OptionalBoolFalse
	if skipTLS {
		skipTLSVerify = types.OptionalBoolTrue
	}
	ctx := &types.SystemContext{
		RegistriesDirPath:           "",
		ArchitectureChoice:          "",
		OSChoice:                    "",
		VariantChoice:               "",
		BigFilesTemporaryDir:        "", //*globalArgs.cache + "/tmp",
		DockerInsecureSkipTLSVerify: skipTLSVerify,
	}
	if registriesConfigPath != "" {
		ctx.SystemRegistriesConfPath = registriesConfigPath
	} else {
		err := environment.UpdateRegistriesConf(ctx)
		if err != nil {
			// log and ignore
			klog.Warningf("unable to load registries.conf from environment variables: %v", err)

		}
	}
	return ctx
}
