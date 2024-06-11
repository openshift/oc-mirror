package clusterresources

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	confv1 "github.com/openshift/api/config/v1"
	ofv1alpha1 "github.com/openshift/oc-mirror/v2/internal/pkg/api/operator-framework/v1alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	updateservicev1 "github.com/openshift/oc-mirror/v2/internal/pkg/clusterresources/updateservice/v1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

const (
	hashTruncLen int = 12
)

func New(log clog.PluggableLoggerInterface,
	workingDir string,
	conf v2alpha1.ImageSetConfiguration,
) GeneratorInterface {
	return &ClusterResourcesGenerator{Log: log, WorkingDir: workingDir, Config: conf}
}

type ClusterResourcesGenerator struct {
	Log        clog.PluggableLoggerInterface
	WorkingDir string
	Config     v2alpha1.ImageSetConfiguration
}

type imageMirrorsGeneratorMode int
type mirrorCategory int

type categorizedMirrors struct {
	category mirrorCategory
	mirrors  map[string][]confv1.ImageMirror
}

const (
	DigestsOnlyMode = iota
	TagsOnlyMode

	releaseCategory = iota
	operatorCategory
	genericCategory

	idmsFileName = "idms-oc-mirror.yaml"
	itmsFileName = "itms-oc-mirror.yaml"
)

func (o *ClusterResourcesGenerator) IDMS_ITMSGenerator(allRelatedImages []v2alpha1.CopyImageSchema, forceRepositoryScope bool) error {
	if len(allRelatedImages) == 0 {
		o.Log.Info("📄 Nothing mirrored. Skipping IDMS and ITMS files generation.")
		return nil
	}

	// byDigestMirrors
	byDigestMirrors, err := o.generateImageMirrors(allRelatedImages, DigestsOnlyMode, forceRepositoryScope)
	if err != nil {
		return err
	}

	//byTagMirrors
	byTagMirrors, err := o.generateImageMirrors(allRelatedImages, TagsOnlyMode, forceRepositoryScope)
	if err != nil {
		return err
	}
	// if byTagMirrors not empty
	if len(byDigestMirrors) > 0 {
		o.Log.Info("📄 Generating IDMS file...")
		idmsList, err := o.generateIDMS(byDigestMirrors)
		if err != nil {
			return err
		}

		err = writeMirrorSet(idmsList, o.WorkingDir, idmsFileName, o.Log)
		if err != nil {
			return err
		}
	} else {
		o.Log.Info("📄 No images by digests were mirrored. Skipping IDMS generation.")
	}
	// if byTagMirrors not empty
	if len(byTagMirrors) > 0 {
		o.Log.Info("📄 Generating ITMS file...")
		itmsList, err := o.generateITMS(byTagMirrors)
		if err != nil {
			return err
		}
		err = writeMirrorSet(itmsList, o.WorkingDir, itmsFileName, o.Log)
		if err != nil {
			return err
		}
	} else {
		o.Log.Info("📄 No images by tag were mirrored. Skipping ITMS generation.")
	}
	return nil
}

func (o *ClusterResourcesGenerator) generateITMS(mirrorsByCategory []categorizedMirrors) ([]confv1.ImageTagMirrorSet, error) {
	// fill itmsList content
	itmsList := make([]confv1.ImageTagMirrorSet, len(mirrorsByCategory))
	for index, catMirrors := range mirrorsByCategory {
		itmsList[index] = confv1.ImageTagMirrorSet{
			TypeMeta: metav1.TypeMeta{
				APIVersion: confv1.GroupVersion.String(),
				Kind:       "ImageTagMirrorSet",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "itms-" + catMirrors.category.toString() + "-0",
			},
			Spec: confv1.ImageTagMirrorSetSpec{
				ImageTagMirrors: []confv1.ImageTagMirrors{},
			},
		}
		for source, imgMirrors := range catMirrors.mirrors {
			itm := confv1.ImageTagMirrors{
				Source:  source,
				Mirrors: imgMirrors,
			}
			itmsList[index].Spec.ImageTagMirrors = append(itmsList[index].Spec.ImageTagMirrors, itm)
		}
	}

	return itmsList, nil
}

func writeMirrorSet[T confv1.ImageDigestMirrorSet | confv1.ImageTagMirrorSet](mirrorSetsList []T, workingDir, fileName string, log clog.PluggableLoggerInterface) error {
	msFilePath := filepath.Join(workingDir, clusterResourcesDir, fileName)
	msAggregation := []byte{}
	var err error
	for _, ms := range mirrorSetsList {
		// Create an unstructured object for removing creationTimestamp
		unstructuredObj := unstructured.Unstructured{}
		unstructuredObj.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&ms)
		if err != nil {
			return fmt.Errorf("error while sanitizing the catalogSource object prior to marshalling: %v", err)
		}
		delete(unstructuredObj.Object["metadata"].(map[string]interface{}), "creationTimestamp")

		msBytes, err := yaml.Marshal(unstructuredObj.Object)
		if err != nil {
			return err
		}
		msAggregation = append(msAggregation, []byte("---\n")...)
		msAggregation = append(msAggregation, msBytes...)
	}
	// save IDMS struct to file
	if _, err := os.Stat(msFilePath); errors.Is(err, os.ErrNotExist) {
		log.Debug("%s does not exist, creating it", msFilePath)
		err := os.MkdirAll(filepath.Dir(msFilePath), 0755)
		if err != nil {
			return err
		}
		log.Debug("%s dir created", filepath.Dir(msFilePath))
	}
	msFile, err := os.Create(msFilePath)
	if err != nil {
		return err
	}
	log.Info("%s file created", msFilePath)

	defer msFile.Close()

	_, err = msFile.Write(msAggregation)
	return err
}

func (o *ClusterResourcesGenerator) CatalogSourceGenerator(allRelatedImages []v2alpha1.CopyImageSchema) error {
	if len(o.Config.Mirror.Operators) == 0 {
		o.Log.Info("📄 No catalogs mirrored. Skipping CatalogSource file generation.")
		return nil
	}

	o.Log.Info("📄 Generating CatalogSource file...")
	for _, copyImage := range allRelatedImages {
		if copyImage.Type == v2alpha1.TypeOperatorCatalog {
			// check if ImageSetConfig contains a CatalogSourceTemplate for this catalog, and use it
			template := o.getCSTemplate(copyImage.Origin)
			err := o.generateCatalogSource(copyImage.Destination, template)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *ClusterResourcesGenerator) getCSTemplate(catalogRef string) string {
	for _, op := range o.Config.ImageSetConfigurationSpec.Mirror.Operators {
		if strings.Contains(catalogRef, op.Catalog) {
			return op.TargetCatalogSourceTemplate
		}
	}
	return ""
}

func (o *ClusterResourcesGenerator) generateCatalogSource(catalogRef string, catalogSourceTemplateFile string) error {

	catalogSpec, err := image.ParseRef(catalogRef)
	if err != nil {
		return err
	}

	var csSuffix string
	if catalogSpec.IsImageByDigestOnly() {
		if len(catalogSpec.Digest) >= hashTruncLen {
			csSuffix = catalogSpec.Digest[:hashTruncLen]
		} else {
			csSuffix = catalogSpec.Digest
		}
	} else {
		tag := catalogSpec.Tag
		if len(tag) >= hashTruncLen {
			csSuffix = strings.Map(toRFC1035, tag[:hashTruncLen])
		} else {
			csSuffix = strings.Map(toRFC1035, tag)
		}
	}

	if csSuffix == "" {
		csSuffix = "0" // default value
	}

	pathComponents := strings.Split(catalogSpec.PathComponent, "/")
	catalogRepository := pathComponents[len(pathComponents)-1]
	catalogSourceName := "cs-" + catalogRepository + "-" + csSuffix
	errs := validation.IsDNS1035Label(catalogSourceName)
	if len(errs) != 0 && !isValidRFC1123(catalogSourceName) {
		return fmt.Errorf("error creating catalog source name: %s", strings.Join(errs, ", "))
	}

	var obj ofv1alpha1.CatalogSource
	generateWithoutTemplate := false
	if catalogSourceTemplateFile != "" {
		obj, err = catalogSourceContentFromTemplate(catalogSourceTemplateFile, catalogSourceName, catalogSpec.Reference)
		if err != nil {
			generateWithoutTemplate = true
			o.Log.Error("error generating catalog source from template. Fall back to generating catalog source without template: %v", err)
		}
	}
	if generateWithoutTemplate || catalogSourceTemplateFile == "" {
		obj = ofv1alpha1.CatalogSource{
			TypeMeta: metav1.TypeMeta{
				APIVersion: ofv1alpha1.GroupName + "/" + ofv1alpha1.GroupVersion,
				Kind:       "CatalogSource",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      catalogSourceName,
				Namespace: "openshift-marketplace",
			},
			Spec: ofv1alpha1.CatalogSourceSpec{
				SourceType: "grpc",
				Image:      catalogSpec.Reference,
			},
		}
	}

	// Create an unstructured object for removing creationTimestamp
	unstructuredObj := unstructured.Unstructured{}
	unstructuredObj.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&obj)
	if err != nil {
		return fmt.Errorf("error while sanitizing the catalogSource object prior to marshalling: %v", err)
	}
	delete(unstructuredObj.Object["metadata"].(map[string]interface{}), "creationTimestamp")

	bytes, err := yaml.Marshal(unstructuredObj.Object)
	if err != nil {
		return fmt.Errorf("unable to marshal CatalogSource yaml: %v", err)
	}

	csFileName := filepath.Join(o.WorkingDir, clusterResourcesDir, catalogSourceName+".yaml")
	// save IDMS struct to file
	if _, err := os.Stat(csFileName); errors.Is(err, os.ErrNotExist) {
		o.Log.Debug("%s does not exist, creating it", csFileName)
		err := os.MkdirAll(filepath.Dir(csFileName), 0755)
		if err != nil {
			return err
		}
		o.Log.Debug("%s dir created", filepath.Dir(csFileName))
	}
	csFile, err := os.Create(csFileName)
	if err != nil {
		return err
	}

	defer csFile.Close()

	_, err = csFile.Write(bytes)
	o.Log.Info("%s file created", csFileName)

	return err
}

func catalogSourceContentFromTemplate(templateFile, catalogSourceName, image string) (ofv1alpha1.CatalogSource, error) {
	// Initializing catalogSource `obj` from template
	var obj ofv1alpha1.CatalogSource
	_, err := os.Stat(templateFile)
	if os.IsNotExist(err) {
		return obj, fmt.Errorf("error during CatalogSource generation using template: targetCatalogSourceTemplate does not exist: %s : %v", templateFile, err)
	}
	if err != nil {
		return obj, fmt.Errorf("error during CatalogSource generation using template: error accessing targetCatalogSourceTemplate file %s: %v", templateFile, err)
	}
	bytesRead, err := os.ReadFile(templateFile)
	if err != nil {
		return obj, fmt.Errorf("error during CatalogSource generation using template: error reading targetCatalogSourceTemplate file %s: %v", templateFile, err)
	}
	err = yaml.Unmarshal(bytesRead, &obj)
	if err != nil {
		return obj, fmt.Errorf("error during CatalogSource generation using template: %s is not a valid catalog source template and could not be unmarshaled: %v", templateFile, err)
	}

	// validate that the catalogSource is grpc, otherwise fail
	if obj.APIVersion != "operators.coreos.com/v1alpha1" {
		return ofv1alpha1.CatalogSource{}, fmt.Errorf("error during CatalogSource generation using template: catalog template does not correspond to the apiVersion operators.coreos.com/v1alpha1 : %s", obj.APIVersion)
	}
	if obj.Kind != "CatalogSource" {
		return ofv1alpha1.CatalogSource{}, fmt.Errorf("error during CatalogSource generation using template: catalog template does not correspond to Kind CatalogSource : %s", obj.Kind)
	}
	if obj.Spec.SourceType != "" && obj.Spec.SourceType != "grpc" {
		return ofv1alpha1.CatalogSource{}, fmt.Errorf("error during CatalogSource generation using template: catalog template is not of sourceType grpc")
	}
	if obj.Spec.ConfigMap != "" {
		return ofv1alpha1.CatalogSource{}, fmt.Errorf("error during CatalogSource generation using template: catalog template should not have a configMap specified")
	}
	// fill obj with the values for this catalog
	obj.Name = catalogSourceName
	obj.Namespace = "openshift-marketplace"
	obj.Spec.SourceType = "grpc"
	obj.Spec.Image = image

	//verify that the resulting obj is a valid CatalogSource object
	_, err = yaml.Marshal(obj)
	if err != nil {
		return ofv1alpha1.CatalogSource{}, fmt.Errorf("error during CatalogSource generation using template: %v", err)
	}

	return obj, nil
}

func (o *ClusterResourcesGenerator) generateIDMS(mirrorsByCategory []categorizedMirrors) ([]confv1.ImageDigestMirrorSet, error) {

	// create a IDMS struct
	idmsList := make([]confv1.ImageDigestMirrorSet, len(mirrorsByCategory))
	for index, catMirrors := range mirrorsByCategory {
		idmsList[index] = confv1.ImageDigestMirrorSet{
			TypeMeta: metav1.TypeMeta{
				APIVersion: confv1.GroupVersion.String(),
				Kind:       "ImageDigestMirrorSet",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "idms-" + catMirrors.category.toString() + "-0",
			},
			Spec: confv1.ImageDigestMirrorSetSpec{
				ImageDigestMirrors: []confv1.ImageDigestMirrors{},
			},
		}
		for source, imgMirrors := range catMirrors.mirrors {
			idm := confv1.ImageDigestMirrors{
				Source:  source,
				Mirrors: imgMirrors,
			}
			idmsList[index].Spec.ImageDigestMirrors = append(idmsList[index].Spec.ImageDigestMirrors, idm)
		}
	}

	return idmsList, nil
}

func (o *ClusterResourcesGenerator) generateImageMirrors(allRelatedImages []v2alpha1.CopyImageSchema, mode imageMirrorsGeneratorMode, forceRepositoryScope bool) ([]categorizedMirrors, error) {
	mirrorsByCategory := make(map[mirrorCategory]categorizedMirrors)
	for _, relatedImage := range allRelatedImages {
		if relatedImage.Origin == "" {
			return nil, fmt.Errorf("unable to generate IDMS/ITMS: original reference for (%s,%s) undetermined", relatedImage.Source, relatedImage.Destination)
		}
		if relatedImage.Type == v2alpha1.TypeCincinnatiGraph || relatedImage.Type == v2alpha1.TypeOperatorCatalog {
			// cincinnati graph images and operator catalog images don't need to be in the IDMS/ITMS file.
			// * cincinnati graph image has been generated from scratch by oc-mirror and will be copied to the destination registry.
			// The updateservice.yaml file will instruct the cluster to use it.
			// * operator catalogs are added to catalog source custom resources, and is consumed by the cluster from there.
			// it therefore doesn't need to be added to IDMS, same as oc-mirror
			// [v1 doesn't add it to ICSP](https://github.com/openshift/oc-mirror/blob/fa0c2caa6a3eb33ed7a7b3350e3b5fc7430bad55/pkg/cli/mirror/mirror.go#L539).
			continue
		}
		srcImgSpec, err := image.ParseRef(relatedImage.Origin)
		if err != nil {
			return nil, fmt.Errorf("unable to generate IDMS/ITMS: %v", err)
		}
		dstImgSpec, err := image.ParseRef(relatedImage.Destination)
		if err != nil {
			return nil, fmt.Errorf("unable to generate IDMS/ITMS: %v", err)
		}
		toBeAdded := true
		switch mode {
		case TagsOnlyMode:
			if srcImgSpec.IsImageByDigestOnly() {
				toBeAdded = false
			}
		case DigestsOnlyMode:
			if !srcImgSpec.IsImageByDigestOnly() {
				toBeAdded = false
			}
		}
		if !toBeAdded {
			continue
		}
		source := ""
		if forceRepositoryScope {
			source = repositoryScope(srcImgSpec)
		} else {
			source = namespaceScope(srcImgSpec)
		}
		mirror := ""
		if forceRepositoryScope {
			mirror = repositoryScope(dstImgSpec)
		} else {
			mirror = namespaceScope(dstImgSpec)
		}

		categoryOfImage := imageTypeToCategory(relatedImage.Type)
		if _, ok := mirrorsByCategory[categoryOfImage]; !ok {
			mirrorsByCategory[categoryOfImage] = categorizedMirrors{
				category: categoryOfImage,
				mirrors:  make(map[string][]confv1.ImageMirror),
			}
		}

		mirrors := mirrorsByCategory[categoryOfImage].mirrors
		// add entry to map
		if mirrors[source] == nil {
			mirrors[source] = []confv1.ImageMirror{confv1.ImageMirror(mirror)}
		} else {
			alreadyAdded := false
			for _, m := range mirrors[source] {
				if m == confv1.ImageMirror(mirror) {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				mirrors[source] = append(mirrors[source], confv1.ImageMirror(mirror))
			}
		}
	}
	categorizedMirrorsList := make([]categorizedMirrors, 0, len(mirrorsByCategory))
	for _, mirrors := range mirrorsByCategory {
		categorizedMirrorsList = append(categorizedMirrorsList, mirrors)
	}
	return categorizedMirrorsList, nil
}

func (o *ClusterResourcesGenerator) UpdateServiceGenerator(graphImageRef, releaseImageRef string) error {
	o.Log.Info("📄 Generating UpdateService file...")
	// truncate tag or digest from release image
	// according to https://docs.openshift.com/container-platform/4.14/updating/updating_a_cluster/updating_disconnected_cluster/disconnected-update-osus.html#update-service-create-service-cli_updating-restricted-network-cluster-osus
	releaseImage, err := image.ParseRef(releaseImageRef)
	if err != nil {
		return err
	}
	releaseImageName := releaseImage.Name

	graphImage, err := image.ParseRef(graphImageRef)
	if err != nil {
		return err
	}

	osus := updateservicev1.UpdateService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: updateservicev1.GroupVersion.String(),
			Kind:       updateServiceResourceKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: updateServiceResourceName,
		},
		Spec: updateservicev1.UpdateServiceSpec{
			Replicas:       2,
			Releases:       releaseImageName,
			GraphDataImage: graphImage.Reference,
		},
	}

	// put UpdateService in yaml
	osusBytes, err := yaml.Marshal(osus)
	if err != nil {
		return err
	}
	// creationTimestamp is a struct, omitempty does not apply
	osusBytes = bytes.ReplaceAll(osusBytes, []byte("  creationTimestamp: null\n"), []byte(""))

	// save UpdateService struct to file
	osusPath := filepath.Join(o.WorkingDir, clusterResourcesDir, updateServiceFilename)

	if _, err := os.Stat(osusPath); errors.Is(err, os.ErrNotExist) {
		o.Log.Debug("%s does not exist, creating it", osusPath)
		err := os.MkdirAll(filepath.Dir(osusPath), 0755)
		if err != nil {
			return err
		}
		o.Log.Debug("%s dir created", filepath.Dir(osusPath))
	}
	osusFile, err := os.Create(osusPath)
	if err != nil {
		return err
	}
	o.Log.Info("%s file created", osusPath)

	defer osusFile.Close()

	_, err = osusFile.Write(osusBytes)
	return err
}

func namespaceScope(imgSpec image.ImageSpec) string {
	pathComponents := strings.Split(imgSpec.PathComponent, "/")
	ns := strings.Join(pathComponents[:len(pathComponents)-1], "/")
	if ns != "" {
		return imgSpec.Domain + "/" + ns
	} else {
		return imgSpec.Domain
	}

}

func repositoryScope(imgSpec image.ImageSpec) string {
	return imgSpec.Name
}

func (m mirrorCategory) toString() string {
	switch m {
	case releaseCategory:
		return "release"
	case operatorCategory:
		return "operator"
	case genericCategory:
		return "generic"
	default:
		return "generic"
	}
}

func imageTypeToCategory(imageType v2alpha1.ImageType) mirrorCategory {
	switch imageType {
	case v2alpha1.TypeCincinnatiGraph:
		return releaseCategory
	case v2alpha1.TypeGeneric:
		return genericCategory
	case v2alpha1.TypeOCPRelease:
		return releaseCategory
	case v2alpha1.TypeOCPReleaseContent:
		return releaseCategory
	case v2alpha1.TypeOperatorBundle:
		return operatorCategory
	case v2alpha1.TypeOperatorCatalog:
		return operatorCategory
	case v2alpha1.TypeOperatorRelatedImage:
		return operatorCategory
	case v2alpha1.TypeInvalid:
		return genericCategory
	default:
		return genericCategory
	}
}

func isValidRFC1123(name string) bool {
	// Regular expression to match RFC1123 compliant names
	rfc1123Regex := "^[a-zA-Z0-9][-a-zA-Z0-9]*[a-zA-Z0-9]$"
	match, _ := regexp.MatchString(rfc1123Regex, name)
	return match && len(name) <= 63
}

func toRFC1035(r rune) rune {
	r = unicode.ToLower(r)
	switch {
	case r >= 'a' && r <= 'z':
		return r
	case r >= '0' && r <= '9':
		return r
	case r == '-':
		return r
	default:
		// convert unacceptable character
		return '-'
	}
}
