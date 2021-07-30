package archive

import (
	"os"
	"reflect"
	"testing"
)

func Test_Verify_Archive(t *testing.T) {

	source := "../../testdata/archives/testbundle.tar.gz"
	hash := "../../testdata/archives/testsum.txt"

	file, err := os.Stat(source)

	if err != nil {
		t.Errorf("Invalid path to test file %s: %v", source, err)
	}

	a, err := NewArchiver()

	if err != nil {
		t.Errorf("Cannot create archiver for file %s", file.Name())
	}

	if err = VerifyArchive(a, source, hash); err != nil {
		t.Errorf("Failed to verify for %s: %v", source, err)
	}
}

func Test_Map_Checksum(t *testing.T) {

	known := make(map[string]string)

	known["testbundle.tar.gz"] = "ca354ab04e22f7f8d72bb7012ab87c36cfb9537da4e9b927700bff7d02b8a72d"

	hash := "../../testdata/archives/testsum.txt"

	_, err := os.Stat(hash)

	if err != nil {
		t.Errorf("Invalid path to test file %s: %v", hash, err)
	}

	actual, err := MapChecksum(hash)

	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(actual, known) {
		t.Errorf("Expected '%v', got '%v'", known, actual)
	}
}

func Test_Checksum_Generation(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    string
		wantErr bool
	}{
		{
			name:   "testing checksum veriftion",
			source: "../../testdata/archives/src/test.txt",
			want:   "b1ab25c55913c95cc6913f1dbce9bef185ebf00a64553a8ef194193e52ea5015",
		},
	}
	for _, tt := range tests {

		sum, err := generateCheckSum(tt.source)

		if err != nil {
			t.Errorf("Test %s: Failed to create generate %s: %v", tt.name, tt.want, err)
		}

		if !tt.wantErr {
			if !reflect.DeepEqual(sum, tt.want) {
				t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, sum)
			}
		}
	}
}
