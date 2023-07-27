package diff

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/config"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

const (
	metadataFile string = ".metadata.toml"
	workingDir   string = "working-dir/"
	errMsg       string = "[DeleteImages] "
)

type DiffInterface interface {
	DeleteImages(ctx context.Context) error
	CheckDiff(prevCfg v1alpha2.ImageSetConfiguration) (bool, error)
	GetAllMetadata(dir string) (SequenceSchema, v1alpha2.ImageSetConfiguration, error)
	WriteMetadata(dir, dest string, sch SequenceSchema, cfg v1alpha2.ImageSetConfiguration) error
}

func New(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
) DiffInterface {
	return &DiffCollector{Log: log, Config: config, Opts: opts, Mirror: mirror}
}

type DiffCollector struct {
	Log    clog.PluggableLoggerInterface
	Mirror mirror.MirrorInterface
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
}

type DiffSchema struct {
	from  string
	found bool
}

func (o *DiffCollector) DeleteImages(ctx context.Context) error {

	metadata, prevCfg, err := o.GetAllMetadata(o.Opts.Global.Dir)
	if err != nil {
		return fmt.Errorf(errMsg+"metadata %v ", err)
	}

	res, err := o.CheckDiff(prevCfg)
	if err != nil {
		return fmt.Errorf(errMsg+"check diff %v ", err)
	}

	if res && !o.Opts.Global.Force {
		imgs, err := calculateDiff(prevCfg, o.Config)
		if err != nil {
			return err
		}

		imgsToDelete, err := batchWorkerConverter(o.Opts.Destination, imgs)
		if err != nil {
			return err
		}

		mirror := mirror.NewMirrorDelete()
		// TODO: consider batch processing this
		for _, s := range imgsToDelete {
			err := mirror.DeleteImage(ctx, s, &o.Opts)
			if err != nil {
				return err
			}
		}
	} else {
		o.Log.Info("[DeleteImages] no images found to delete")
	}

	err = o.WriteMetadata(o.Opts.Global.Dir, o.Opts.Destination, metadata, o.Config)
	if err != nil {
		return err
	}

	return nil
}

func (o *DiffCollector) CheckDiff(prevCfg v1alpha2.ImageSetConfiguration) (bool, error) {

	if !reflect.DeepEqual(o.Config, prevCfg) {
		return true, nil
	}
	return false, nil
}

func (o *DiffCollector) GetAllMetadata(dir string) (SequenceSchema, v1alpha2.ImageSetConfiguration, error) {
	metadata, err := readMetaData(dir)
	if err != nil {
		return SequenceSchema{}, v1alpha2.ImageSetConfiguration{}, err
	}
	_, isc, err := getPreviousISC(metadata)
	if err != nil {
		return SequenceSchema{}, v1alpha2.ImageSetConfiguration{}, err
	}
	prevCfg, err := config.ReadConfig(isc)
	if err != nil {
		return SequenceSchema{}, v1alpha2.ImageSetConfiguration{}, err
	}
	return metadata, prevCfg, nil
}

// writeMetadata
func (o *DiffCollector) WriteMetadata(dir, dest string, sch SequenceSchema, cfg v1alpha2.ImageSetConfiguration) error {

	for i := range sch.Sequence.Item {
		sch.Sequence.Item[i].Current = false
	}

	newItem := &Item{
		Value:          len(sch.Sequence.Item),
		Current:        true,
		Imagesetconfig: dir + "/.imagesetconfig-" + strconv.Itoa(len(sch.Sequence.Item)) + ".yaml",
		Timestamp:      time.Now().Unix(),
		Destination:    dest,
	}

	sch.Sequence.Item = append(sch.Sequence.Item, *newItem)
	f, err := os.Create(dir + "/" + metadataFile)
	if err != nil {
		// failed to create/open the file
		return err
	}
	if err := toml.NewEncoder(f).Encode(sch); err != nil {
		// failed to encode
		return err
	}
	if err := f.Close(); err != nil {
		// failed to close the file
		return err
	}

	data, err := os.ReadFile(o.Opts.Global.ConfigPath)
	if err != nil {
		return err
	}
	err = os.WriteFile(newItem.Imagesetconfig, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// calculateDiff
func calculateDiff(prevConfig, cfg v1alpha2.ImageSetConfiguration) (map[string]DiffSchema, error) {

	images := make(map[string]DiffSchema)

	// iterate through each object find the differences
	// if the image object is found its set to true
	// else its set to false, we can then iterate through
	// the map to verify all the false objects  that are from previous
	// runs should be deleted

	for _, pr := range prevConfig.Mirror.Platform.Channels {
		images[pr.Name] = DiffSchema{from: "previous", found: false}
		for _, r := range cfg.Mirror.Platform.Channels {
			images[r.Name] = DiffSchema{from: "current", found: false}
			if reflect.DeepEqual(r, pr) {
				images[pr.Name] = DiffSchema{from: "both", found: true}
			}
		}
	}
	for _, prOp := range prevConfig.Mirror.Operators {
		images[prOp.Catalog] = DiffSchema{from: "previous", found: false}
		for _, op := range cfg.Mirror.Operators {
			images[op.Catalog] = DiffSchema{from: "current", found: false}
			for _, prevPkg := range prOp.Packages {
				images[prevPkg.Name] = DiffSchema{from: "previous", found: false}
				for _, pkg := range op.Packages {
					images[pkg.Name] = DiffSchema{from: "current", found: false}
					if reflect.DeepEqual(pkg, prevPkg) {
						images[prevPkg.Name] = DiffSchema{from: "both", found: true}
					}
				}
			}
			if reflect.DeepEqual(op, prOp) {
				images[prOp.Catalog] = DiffSchema{from: "both", found: true}
			}
		}
	}

	for _, prAi := range prevConfig.Mirror.AdditionalImages {
		images[prAi.Name] = DiffSchema{from: "previous", found: false}
		for _, ai := range cfg.Mirror.AdditionalImages {
			images[ai.Name] = DiffSchema{from: "current", found: false}
			if reflect.DeepEqual(ai, prAi) {
				images[prAi.Name] = DiffSchema{from: "both", found: true}
			}
		}
	}
	return images, nil
}

// batchWorkerConverter - needs work , have to consider oci format
// and release image channels
func batchWorkerConverter(destination string, imgs map[string]DiffSchema) ([]string, error) {
	var result []string
	for k, v := range imgs {
		if !v.found && v.from == "previous" {
			i, err := customImageParser(k)
			if err != nil {
				return result, err
			}
			src := destination + i.Namespace + "/" + i.Component
			result = append(result, src)
		}
	}
	return result, nil
}

// getPreviousISC
func getPreviousISC(metadata SequenceSchema) (string, string, error) {
	var isc, dest string
	for _, item := range metadata.Sequence.Item {
		if item.Current {
			isc = item.Imagesetconfig
			dest = item.Destination
		}
	}
	if len(isc) == 0 {
		return "", "", fmt.Errorf("no current imagesetconfig found")
	}
	return dest, isc, nil
}

// readMetaData
func readMetaData(dir string) (SequenceSchema, error) {
	var schema SequenceSchema
	if _, err := toml.DecodeFile(dir+"/"+metadataFile, &schema); err != nil {
		return SequenceSchema{}, err
	}
	return schema, nil
}

// customImageParser
func customImageParser(image string) (*v1alpha3.ImageRefSchema, error) {
	var irs *v1alpha3.ImageRefSchema
	var component string
	parts := strings.Split(image, "/")
	if len(parts) < 3 {
		return irs, fmt.Errorf("[customImageParser] image url seems to be wrong %s ", image)
	}
	if strings.Contains(parts[2], "@") {
		component = strings.Split(parts[2], "@")[0]
	} else {
		component = parts[2]
	}
	irs = &v1alpha3.ImageRefSchema{Repository: parts[0], Namespace: parts[1], Component: component}
	return irs, nil
}
