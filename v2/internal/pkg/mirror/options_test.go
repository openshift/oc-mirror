package mirror

import (
	"testing"

	"github.com/containers/common/pkg/flag"
	//"github.com/containers/image/v5/types"
	"github.com/spf13/pflag"
)

func TestOptionsNewContext(t *testing.T) {

	global := &GlobalOptions{SecurePolicy: false}

	_, sharedOpts := SharedImageFlags()
	_, deprecatedTLSVerifyOpt := DeprecatedTLSVerifyFlags()
	_, dockerImageOpts := dockerImageFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "", "")

	_, err := dockerImageOpts.NewSystemContext()
	if err != nil {
		t.Fatal("should not fail")
	}

	opstr := flag.OptionalString{}
	o := flag.NewOptionalStringValue(&opstr)
	_ = o.Set("test")
	dockerImageOpts.authFilePath = opstr
	_, err = dockerImageOpts.NewSystemContext()
	if err != nil {
		t.Fatal("should not fail")
	}

	fs := pflag.FlagSet{}
	fs.BoolVar(&dockerImageOpts.TlsVerify, "tls-verify", false, "whatever")
	dep := flag.OptionalBoolFlag(&fs, &dockerImageOpts.deprecatedTLSVerify.tlsVerify, "deprecatated", "whatever")
	_ = dep.Value.Set("true")
	dockerImageOpts.authFilePath = flag.OptionalString{}
	_, err = dockerImageOpts.NewSystemContext()
	if err != nil {
		t.Fatal("should not fail")
	}

	credsop := flag.OptionalString{}
	creds := flag.NewOptionalStringValue(&credsop)
	_ = creds.Set("test:pwd")
	dockerImageOpts.credsOption = credsop
	dockerImageOpts.noCreds = true
	_, err = dockerImageOpts.NewSystemContext()
	if err == nil {
		t.Fatal("should fail")
	}

	user := flag.OptionalString{}
	username := flag.NewOptionalStringValue(&user)
	_ = username.Set("user")
	dockerImageOpts.credsOption = flag.OptionalString{}
	dockerImageOpts.noCreds = true
	dockerImageOpts.userName = user
	_, err = dockerImageOpts.NewSystemContext()
	if err == nil {
		t.Fatal("should fail")
	}

	dockerImageOpts.credsOption = credsop
	dockerImageOpts.noCreds = false
	dockerImageOpts.userName = user
	_, err = dockerImageOpts.NewSystemContext()
	if err == nil {
		t.Fatal("should fail")
	}

	dockerImageOpts.credsOption = flag.OptionalString{}
	dockerImageOpts.noCreds = false
	dockerImageOpts.userName = flag.OptionalString{}
	dockerImageOpts.password = user
	_, err = dockerImageOpts.NewSystemContext()
	if err == nil {
		t.Fatal("should fail")
	}

	dockerImageOpts.credsOption = credsop
	dockerImageOpts.userName = flag.OptionalString{}
	dockerImageOpts.password = flag.OptionalString{}
	_, err = dockerImageOpts.NewSystemContext()
	if err != nil {
		t.Fatal("should not fail")
	}

	dockerImageOpts.credsOption = flag.OptionalString{}
	dockerImageOpts.userName = user
	dockerImageOpts.password = user
	_, err = dockerImageOpts.NewSystemContext()
	if err != nil {
		t.Fatal("should not fail")
	}

	dockerImageOpts.credsOption = flag.OptionalString{}
	dockerImageOpts.userName = flag.OptionalString{}
	dockerImageOpts.password = flag.OptionalString{}
	dockerImageOpts.registryToken = flag.OptionalString{}
	dockerImageOpts.noCreds = true
	_, err = dockerImageOpts.NewSystemContext()
	if err != nil {
		t.Fatal("should not fail")
	}

	dockerImageOpts.credsOption = flag.OptionalString{}
	dockerImageOpts.userName = flag.OptionalString{}
	dockerImageOpts.password = flag.OptionalString{}
	dockerImageOpts.registryToken = user
	dockerImageOpts.noCreds = false
	_, err = dockerImageOpts.NewSystemContext()
	if err != nil {
		t.Fatal("should not fail")
	}

}
