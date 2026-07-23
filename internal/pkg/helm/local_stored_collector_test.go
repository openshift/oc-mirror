package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	helmrepo "helm.sh/helm/v3/pkg/repo"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/folder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

var (
	testChartsDataPath  = filepath.Join(consts.TestFolder, "helm-data", "charts")
	testIndexesDataPath = filepath.Join(consts.TestFolder, "helm-data", "indexes")
)

const (
	testLocalStorageFQDN = "localhost:8888"
	testDest             = consts.DockerProtocol + "myreg:5000/test"
)

var (
	tempChartDir   string
	tempIndexesDir string

	cfg = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{},
		},
	}
)

type MockIndexDownloader struct{}

type MockChartDownloader struct{}

type MockHttpClient struct{}

type testCase struct {
	caseName           string
	mirrorMode         string
	helmConfig         v2alpha1.Helm
	localStorage       string
	dest               string
	generateV1DestTags bool
	expectedResult     []v2alpha1.CopyImageSchema
	expectedError      error
}

func TestHelmImageCollector(t *testing.T) {
	log := clog.New("trace")

	testCases := []testCase{
		{
			caseName:     "local helm chart - MirrorToDisk: should pass",
			mirrorMode:   mirror.MirrorToDisk,
			localStorage: testLocalStorageFQDN,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "podinfo-local", Path: filepath.Join(testChartsDataPath, "podinfo-5.0.0.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "ghcr.io/stefanprodan/podinfo:5.0.0",
					Destination: consts.DockerProtocol + "localhost:8888/stefanprodan/podinfo:5.0.0",
					Origin:      "ghcr.io/stefanprodan/podinfo:5.0.0",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "local helm chart - MirrorToDisk images by tag and digest: should pass",
			mirrorMode:   mirror.MirrorToDisk,
			localStorage: testLocalStorageFQDN,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "ingress-nginx", Path: filepath.Join(testChartsDataPath, "ingress-nginx-4.12.1.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "registry.k8s.io/ingress-nginx/controller@sha256:d2fbc4ec70d8aa2050dd91a91506e998765e86c96f32cffb56c503c9c34eed5b",
					Destination: consts.DockerProtocol + "localhost:8888/ingress-nginx/controller:v1.12.1",
					Origin:      "registry.k8s.io/ingress-nginx/controller:v1.12.1@sha256:d2fbc4ec70d8aa2050dd91a91506e998765e86c96f32cffb56c503c9c34eed5b",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "repositories helm chart - charts included - MirrorToDisk: should pass",
			mirrorMode:   mirror.MirrorToDisk,
			localStorage: testLocalStorageFQDN,
			helmConfig: v2alpha1.Helm{
				Repositories: []v2alpha1.Repository{
					{Name: "podinfo", URL: "https://stefanprodan.github.io/podinfo", Charts: []v2alpha1.Chart{{Name: "podinfo", Version: "5.0.0"}}},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "ghcr.io/stefanprodan/podinfo:5.0.0",
					Destination: consts.DockerProtocol + "localhost:8888/stefanprodan/podinfo:5.0.0",
					Origin:      "ghcr.io/stefanprodan/podinfo:5.0.0",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "repositories helm chart - charts not included - MirrorToDisk: should pass",
			mirrorMode:   mirror.MirrorToDisk,
			localStorage: testLocalStorageFQDN,
			helmConfig: v2alpha1.Helm{
				Repositories: []v2alpha1.Repository{
					{Name: "sbo", URL: "https://redhat-developer.github.io/service-binding-operator-helm-chart/"},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "quay.io/redhat-developer/servicebinding-operator@sha256:16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Destination: consts.DockerProtocol + "localhost:8888/redhat-developer/servicebinding-operator:sha256-16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + "quay.io/redhat-developer/servicebinding-operator@sha256:ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Destination: consts.DockerProtocol + "localhost:8888/redhat-developer/servicebinding-operator:sha256-ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + "quay.io/redhat-developer/servicebinding-operator@sha256:e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Destination: consts.DockerProtocol + "localhost:8888/redhat-developer/servicebinding-operator:sha256-e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + "quay.io/redhat-developer/servicebinding-operator@sha256:30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Destination: consts.DockerProtocol + "localhost:8888/redhat-developer/servicebinding-operator:sha256-30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + "quay.io/redhat-developer/servicebinding-operator@sha256:67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Destination: consts.DockerProtocol + "localhost:8888/redhat-developer/servicebinding-operator:sha256-67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + "quay.io/redhat-developer/servicebinding-operator@sha256:e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Destination: consts.DockerProtocol + "localhost:8888/redhat-developer/servicebinding-operator:sha256-e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + "quay.io/redhat-developer/servicebinding-operator@sha256:f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Destination: consts.DockerProtocol + "localhost:8888/redhat-developer/servicebinding-operator:sha256-f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + "quay.io/redhat-developer/servicebinding-operator@sha256:69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Destination: consts.DockerProtocol + "localhost:8888/redhat-developer/servicebinding-operator:sha256-69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Destination: "docker://localhost:8888/redhat-developer/servicebinding-operator:sha256-cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Destination: "docker://localhost:8888/redhat-developer/servicebinding-operator:sha256-de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:   "local helm chart - MirrorToMirror: should pass",
			mirrorMode: mirror.MirrorToMirror,
			dest:       testDest,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "podinfo-local", Path: filepath.Join(testChartsDataPath, "podinfo-5.0.0.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://ghcr.io/stefanprodan/podinfo:5.0.0",
					Destination: testDest + "/stefanprodan/podinfo:5.0.0",
					Origin:      "ghcr.io/stefanprodan/podinfo:5.0.0",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:   "local helm chart - MirrorToMirror images by tag and digest: should pass",
			mirrorMode: mirror.MirrorToMirror,
			dest:       testDest,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "ingress-nginx", Path: filepath.Join(testChartsDataPath, "ingress-nginx-4.12.1.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://registry.k8s.io/ingress-nginx/controller@sha256:d2fbc4ec70d8aa2050dd91a91506e998765e86c96f32cffb56c503c9c34eed5b",
					Destination: testDest + "/ingress-nginx/controller:v1.12.1",
					Origin:      "registry.k8s.io/ingress-nginx/controller:v1.12.1@sha256:d2fbc4ec70d8aa2050dd91a91506e998765e86c96f32cffb56c503c9c34eed5b",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:   "repositories helm chart - charts included - MirrorToMirror: should pass",
			mirrorMode: mirror.MirrorToMirror,
			dest:       testDest,
			helmConfig: v2alpha1.Helm{
				Repositories: []v2alpha1.Repository{
					{Name: "podinfo", URL: "https://stefanprodan.github.io/podinfo", Charts: []v2alpha1.Chart{{Name: "podinfo", Version: "5.0.0"}}},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://ghcr.io/stefanprodan/podinfo:5.0.0",
					Destination: testDest + "/stefanprodan/podinfo:5.0.0",
					Origin:      "ghcr.io/stefanprodan/podinfo:5.0.0",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:   "repositories helm chart - charts not included - MirrorToMirror: should pass",
			mirrorMode: mirror.MirrorToMirror,
			dest:       testDest,
			helmConfig: v2alpha1.Helm{
				Repositories: []v2alpha1.Repository{
					{Name: "sbo", URL: "https://redhat-developer.github.io/service-binding-operator-helm-chart/"},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/redhat-developer/servicebinding-operator@sha256:de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "local helm chart - diskToMirror: should pass",
			mirrorMode:   mirror.DiskToMirror,
			localStorage: testLocalStorageFQDN,
			dest:         testDest,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "podinfo-local", Path: filepath.Join(testChartsDataPath, "podinfo-5.0.0.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/stefanprodan/podinfo:5.0.0",
					Destination: testDest + "/stefanprodan/podinfo:5.0.0",
					Origin:      "ghcr.io/stefanprodan/podinfo:5.0.0",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "local helm chart - DiskToMirror images by tag and digest: should pass",
			mirrorMode:   mirror.DiskToMirror,
			localStorage: testLocalStorageFQDN,
			dest:         testDest,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "ingress-nginx", Path: filepath.Join(testChartsDataPath, "ingress-nginx-4.12.1.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/ingress-nginx/controller@sha256:d2fbc4ec70d8aa2050dd91a91506e998765e86c96f32cffb56c503c9c34eed5b",
					Destination: testDest + "/ingress-nginx/controller:v1.12.1",
					Origin:      "registry.k8s.io/ingress-nginx/controller:v1.12.1@sha256:d2fbc4ec70d8aa2050dd91a91506e998765e86c96f32cffb56c503c9c34eed5b",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "repositories helm chart - charts included - diskToMirror: should pass",
			mirrorMode:   mirror.DiskToMirror,
			localStorage: testLocalStorageFQDN,
			dest:         testDest,
			helmConfig: v2alpha1.Helm{
				Repositories: []v2alpha1.Repository{
					{Name: "podinfo", URL: "https://stefanprodan.github.io/podinfo", Charts: []v2alpha1.Chart{{Name: "podinfo", Version: "5.0.0"}}},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/stefanprodan/podinfo:5.0.0",
					Destination: testDest + "/stefanprodan/podinfo:5.0.0",
					Origin:      "ghcr.io/stefanprodan/podinfo:5.0.0",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "repositories helm chart - charts not included - diskToMirror: should pass",
			mirrorMode:   mirror.DiskToMirror,
			localStorage: testLocalStorageFQDN,
			dest:         testDest,
			helmConfig: v2alpha1.Helm{
				Repositories: []v2alpha1.Repository{
					{Name: "sbo", URL: "https://redhat-developer.github.io/service-binding-operator-helm-chart/"},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Destination: testDest + "/redhat-developer/servicebinding-operator:sha256-de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "repositories helm chart - charts not included - diskToMirror with generateV1Tags: should pass",
			mirrorMode:   mirror.DiskToMirror,
			localStorage: testLocalStorageFQDN,
			dest:         testDest,
			helmConfig: v2alpha1.Helm{
				Repositories: []v2alpha1.Repository{
					{Name: "sbo", URL: "https://redhat-developer.github.io/service-binding-operator-helm-chart/"},
				},
			},
			generateV1DestTags: true,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:16286ac84ddd521897d92472dae857a4c18479f255b725dfb683bc72df6e0865",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:e4259939a496f292a31b5e57760196d63a8182b999164d93a446da48c4ea24eb",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:30bf7f0f21024bb2e1e4db901b1f5e89ab56e0f3197a919d2bbb670f3fe5223a",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:67c2a2502f59fac1e7ded9ed19b59bbd4e50f5559a13978a87ecd2283b81e067",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:e01016cacae84dfb6eaf7a1022130e7d95e2a8489c38d4d46e4f734848e93849",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:f79f6999a15534dbe56e658caf94fc4b7afb5ceeb7b49f32a60ead06fbd7c3fc",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:69a95c6216ead931e01e4144ae8f4fb7ab35d1f68a14c18f6860a085ccb950f5",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:cc5aab01ddd3744510c480eb4f58b834936a833d36bec5c9c13fb40bbb06c663",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/redhat-developer/servicebinding-operator:sha256-de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Destination: testDest + "/redhat-developer/servicebinding-operator:latest",
					Origin:      "quay.io/redhat-developer/servicebinding-operator@sha256:de1881753e82c51b31e958fcf383cb35b0f70f6ec99d402d42243e595d00c6dd",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "local helm chart with images on deployment env vars- MirrorToDisk: should pass",
			mirrorMode:   mirror.MirrorToDisk,
			localStorage: testLocalStorageFQDN,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "test-mirror-helm", Path: filepath.Join(testChartsDataPath, "test-mirror-helm-0.3.0.tgz"), ImagePaths: []string{"{.spec.template.spec.containers[*].env[*].value}"}},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://quay.io/nginx/nginx-ingress:latest",
					Destination: "docker://localhost:8888/nginx/nginx-ingress:latest",
					Origin:      "quay.io/nginx/nginx-ingress:latest",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/prometheus/prometheus:latest",
					Destination: "docker://localhost:8888/prometheus/prometheus:latest",
					Origin:      "quay.io/prometheus/prometheus:latest",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "local helm chart - aliased sub-chart - MirrorToDisk: should pass",
			mirrorMode:   mirror.MirrorToDisk,
			localStorage: testLocalStorageFQDN,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "my-chart-with-subchart-alias", Path: filepath.Join(testChartsDataPath, "my-chart-with-subchart-alias-0.1.0.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://quay.io/rhdh-community/rhdh:next",
					Destination: "docker://localhost:8888/rhdh-community/rhdh:next",
					Origin:      "quay.io/rhdh-community/rhdh:next",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://nginx:1.16.0",
					Destination: "docker://localhost:8888/nginx:1.16.0",
					Origin:      "nginx:1.16.0",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/fedora/postgresql-15:latest",
					Destination: "docker://localhost:8888/fedora/postgresql-15:latest",
					Origin:      "quay.io/fedora/postgresql-15:latest",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:   "local helm chart - aliased sub-chart - MirrorToMirror: should pass",
			mirrorMode: mirror.MirrorToMirror,
			dest:       testDest,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "my-chart-with-subchart-alias", Path: filepath.Join(testChartsDataPath, "my-chart-with-subchart-alias-0.1.0.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://quay.io/rhdh-community/rhdh:next",
					Destination: testDest + "/rhdh-community/rhdh:next",
					Origin:      "quay.io/rhdh-community/rhdh:next",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://nginx:1.16.0",
					Destination: testDest + "/nginx:1.16.0",
					Origin:      "nginx:1.16.0",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      "docker://quay.io/fedora/postgresql-15:latest",
					Destination: testDest + "/fedora/postgresql-15:latest",
					Origin:      "quay.io/fedora/postgresql-15:latest",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
		{
			caseName:     "local helm chart - aliased sub-chart - DiskToMirror: should pass",
			mirrorMode:   mirror.DiskToMirror,
			localStorage: testLocalStorageFQDN,
			dest:         testDest,
			helmConfig: v2alpha1.Helm{
				Local: []v2alpha1.Chart{
					{Name: "my-chart-with-subchart-alias", Path: filepath.Join(testChartsDataPath, "my-chart-with-subchart-alias-0.1.0.tgz")},
				},
			},
			generateV1DestTags: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/rhdh-community/rhdh:next",
					Destination: testDest + "/rhdh-community/rhdh:next",
					Origin:      "quay.io/rhdh-community/rhdh:next",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/nginx:1.16.0",
					Destination: testDest + "/nginx:1.16.0",
					Origin:      "nginx:1.16.0",
					Type:        v2alpha1.TypeHelmImage,
				},
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/fedora/postgresql-15:latest",
					Destination: testDest + "/fedora/postgresql-15:latest",
					Origin:      "quay.io/fedora/postgresql-15:latest",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
			expectedError: nil,
		},
	}

	tempDir := t.TempDir()
	workingDir, err := prepareFolder(tempDir)
	assert.NoError(t, err)

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			_, srcOpts := mirror.ImageSrcFlags(nil, nil, nil, "src-", "screds")
			opts := mirror.CopyOptions{
				Mode:             testCase.mirrorMode,
				Global:           &mirror.GlobalOptions{WorkingDir: workingDir},
				LocalStorageFQDN: testCase.localStorage,
				Destination:      testCase.dest,
				SrcImage:         srcOpts,
			}

			cfg.Mirror.Helm = testCase.helmConfig

			ctx := context.Background()

			mockIndexDownloader := MockIndexDownloader{}
			mockChartDownloader := MockChartDownloader{}
			mockHttpClient := MockHttpClient{}

			helmCollector := New(log, cfg, opts, mockIndexDownloader, mockChartDownloader, mockHttpClient)
			if testCase.generateV1DestTags {
				helmCollector = WithV1Tags(helmCollector)
			}
			if testCase.mirrorMode == mirror.DiskToMirror {
				prepareDiskToMirror(testCase)
			}

			imgs, err := helmCollector.HelmImageCollector(ctx)

			if testCase.expectedError == nil {
				assert.NoError(t, err)
			}

			if len(testCase.expectedResult) > 0 {
				assert.NotEmpty(t, imgs.AllImages)
				assert.ElementsMatch(t, testCase.expectedResult, imgs.AllImages)
			}
		})
	}
}

// TestResolveChartPath verifies that resolveChartPath tolerates mismatches
// between the version string in the ImageSetConfiguration and the "v" prefix
// that a Helm repository may embed in its tarball filenames.
func TestResolveChartPath(t *testing.T) {
	dir := t.TempDir()

	// chartA-1.0.0.tgz  – version stored without "v"
	// chartB-v2.0.0.tgz – version stored with "v"
	chartAFile := filepath.Join(dir, "chartA-1.0.0.tgz")
	chartBFile := filepath.Join(dir, "chartB-v2.0.0.tgz")
	assert.NoError(t, os.WriteFile(chartAFile, []byte("placeholder"), 0600))
	assert.NoError(t, os.WriteFile(chartBFile, []byte("placeholder"), 0600))

	// resolveChartPath calls lsc.Log.Debug; initialise the global so the
	// helper does not panic.
	lsc = &LocalStorageCollector{Log: clog.New("trace")}

	tests := []struct {
		name      string
		chartName string
		version   string
		wantPath  string
		wantErr   bool
	}{
		{
			name:      "exact match – no v prefix",
			chartName: "chartA",
			version:   "1.0.0",
			wantPath:  chartAFile,
		},
		{
			name:      "config omits v; file on disk has v prefix",
			chartName: "chartB",
			version:   "2.0.0",
			wantPath:  chartBFile,
		},
		{
			name:      "config has v; file on disk has no v prefix",
			chartName: "chartA",
			version:   "v1.0.0",
			wantPath:  chartAFile,
		},
		{
			name:      "neither candidate exists returns error",
			chartName: "missing",
			version:   "9.9.9",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveChartPath(dir, tc.chartName, tc.version)
			if tc.wantErr {
				assert.Error(t, err)
				// Both attempted candidate paths must appear in the error so
				// callers can diagnose which filenames were probed.
				assert.Contains(t, err.Error(), fmt.Sprintf("%s-%s.tgz", tc.chartName, tc.version))
				altVersion := "v" + tc.version
				if strings.HasPrefix(tc.version, "v") {
					altVersion = strings.TrimPrefix(tc.version, "v")
				}
				assert.Contains(t, err.Error(), fmt.Sprintf("%s-%s.tgz", tc.chartName, altVersion))
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantPath, got)
		})
	}
}

// TestHelmImageCollectorVPrefixDiskToMirror ensures that the disk-to-mirror
// collector succeeds even when the chart tarball on disk was saved by the Helm
// downloader using a "v"-prefixed version (e.g. podinfo-v5.0.0.tgz) while the
// ImageSetConfiguration specifies the version without the prefix (5.0.0), or
// vice-versa.
func TestHelmImageCollectorVPrefixDiskToMirror(t *testing.T) {
	log := clog.New("trace")
	ctx := context.Background()

	tests := []struct {
		name           string
		configVersion  string
		diskVersion    string
		expectedImages []v2alpha1.CopyImageSchema
	}{
		{
			name:          "config without v; disk file has v prefix",
			configVersion: "5.0.0",
			diskVersion:   "v5.0.0",
			expectedImages: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/stefanprodan/podinfo:5.0.0",
					Destination: testDest + "/stefanprodan/podinfo:5.0.0",
					Origin:      "ghcr.io/stefanprodan/podinfo:5.0.0",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
		},
		{
			name:          "config with v; disk file has no v prefix",
			configVersion: "v5.0.0",
			diskVersion:   "5.0.0",
			expectedImages: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + testLocalStorageFQDN + "/stefanprodan/podinfo:5.0.0",
					Destination: testDest + "/stefanprodan/podinfo:5.0.0",
					Origin:      "ghcr.io/stefanprodan/podinfo:5.0.0",
					Type:        v2alpha1.TypeHelmImage,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workingDir, err := prepareFolder(t.TempDir())
			assert.NoError(t, err)

			helmCfg := v2alpha1.Helm{
				Repositories: []v2alpha1.Repository{
					{
						Name: "podinfo",
						URL:  "https://stefanprodan.github.io/podinfo",
						Charts: []v2alpha1.Chart{
							{Name: "podinfo", Version: tc.configVersion},
						},
					},
				},
			}

			testCfg := v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{Helm: helmCfg},
				},
			}

			_, srcOpts := mirror.ImageSrcFlags(nil, nil, nil, "src-", "screds")
			opts := mirror.CopyOptions{
				Mode:             mirror.DiskToMirror,
				Global:           &mirror.GlobalOptions{WorkingDir: workingDir},
				LocalStorageFQDN: testLocalStorageFQDN,
				Destination:      testDest,
				SrcImage:         srcOpts,
			}

			New(log, testCfg, opts, nil, nil, nil)

			// Place the chart on disk using the downloader-supplied version
			// string (which may differ from what the config specifies).
			diskFile := filepath.Join(tempChartDir, fmt.Sprintf("podinfo-%s.tgz", tc.diskVersion))
			assert.NoError(t, copy.Copy(filepath.Join(testChartsDataPath, "podinfo-5.0.0.tgz"), diskFile))

			helmCollector := lsc
			imgs, err := helmCollector.HelmImageCollector(ctx)

			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedImages, imgs.AllImages)
		})
	}
}

func prepareDiskToMirror(testCase testCase) error {
	for _, repo := range testCase.helmConfig.Repositories {
		var err error
		var charts []v2alpha1.Chart
		charts = repo.Charts

		if charts == nil {
			namespace := getNamespaceFromURL(repo.URL)
			copyIndex(namespace)

			charts, err = getChartsFromIndex(repo.URL, helmrepo.IndexFile{})
			if err != nil {
				return err
			}
		}

		for _, chart := range charts {
			copyChart(chart.Name, chart.Version)
		}
	}

	return nil
}

func copyChart(ref, version string) string {
	tgzFileName := fmt.Sprintf("%s-%s.tgz", path.Base(ref), version)
	copy.Copy(filepath.Join(testChartsDataPath, tgzFileName), filepath.Join(tempChartDir, tgzFileName))

	return tgzFileName
}

func copyIndex(namespace string) {
	copy.Copy(filepath.Join(testIndexesDataPath, namespace, helmIndexFile), filepath.Join(tempIndexesDir, namespace, helmIndexFile))
}

func (m MockIndexDownloader) DownloadIndexFile() (string, error) {
	return "", nil
}

func (m MockChartDownloader) DownloadTo(ref, version, dest string) (string, any, error) {
	tgzFileName := copyChart(ref, version)

	return filepath.Join(tempChartDir, tgzFileName), "", nil
}

func (m MockHttpClient) Get(url string) (resp *http.Response, err error) {
	ns := getNamespaceFromURL(url)

	response := http.Response{StatusCode: http.StatusOK}

	data, err := os.ReadFile(filepath.Join(testIndexesDataPath, ns, helmIndexFile))
	if err != nil {
		return &http.Response{StatusCode: http.StatusInternalServerError}, err
	}

	response.Body = io.NopCloser(bytes.NewReader(data))

	return &response, nil
}

func prepareFolder(tempDir string) (string, error) {
	workingDir := filepath.Join(tempDir, "working-dir")
	tempChartDir = filepath.Join(workingDir, helmDir, helmChartDir)
	tempIndexesDir = filepath.Join(workingDir, helmDir, helmIndexesDir)

	if err := folder.CreateFolders(tempChartDir, tempIndexesDir); err != nil {
		return "", err
	}

	return workingDir, nil
}
