package bundle

// CreateFull performs all tasks in creating full imagesets
func CreateFull(rootDir string) error {
	validate, err := validateCreateDir(rootDir)
	if validate == false {
		return err
	} else {
		// Read the bundle-config.yaml
		config, err := readBundleConfig(rootDir)
		if err != nil {
			return err
		} else {
			meta, err := readMeta(rootDir)
			if err != nil {
				return err
			} else {
				if &config.Mirror.Ocp != nil {
					GetReleases(meta, rootDir)
				}
			}
		}
	}
	//if &config.Mirror.Operators != nil {
	//GetOperators(*config, rootDir)
	//}
	//if &config.Mirror.Samples != nil {
	//GetSamples(*config, rootDir)
	//}
	//if &config.Mirror.AdditionalImages != nil {
	//	getAdditional(*config, rootDir)
	//}
	return nil
}

// CreateDiff performs all tasks in creating differential imagesets
//func CreateDiff(rootDir string) error {
//    return err
//}

//func downloadObjects() {
//
//}
