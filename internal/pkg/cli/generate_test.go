package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    generateOptions
		args    []string
		host    string
		wantErr string
	}{
		{name: "ok", opts: generateOptions{outputType: "both"}, args: []string{"docker://reg.example.com:5000"}, host: "reg.example.com:5000"},
		{name: "missing arg", opts: generateOptions{outputType: "both"}, wantErr: "registry argument is required"},
		{name: "no protocol", opts: generateOptions{outputType: "both"}, args: []string{"reg.example.com"}, wantErr: "docker://"},
		{name: "path in arg", opts: generateOptions{outputType: "both"}, args: []string{"docker://reg.example.com/team"}, wantErr: "--prefix"},
		{name: "bad type", opts: generateOptions{outputType: "xml"}, args: []string{"docker://r"}, wantErr: "--output-type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := tt.opts.validate(tt.args)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.host, host)
		})
	}
}
