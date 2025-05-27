package release

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"

	// nolint
	"golang.org/x/crypto/openpgp"
)

type SignatureSchema struct {
	Log    clog.PluggableLoggerInterface
	Config v2alpha1.ImageSetConfiguration
	Opts   mirror.CopyOptions
	pgpKey string
}

func NewSignatureClient(log clog.PluggableLoggerInterface, config v2alpha1.ImageSetConfiguration, opts mirror.CopyOptions) SignatureInterface {
	var pgp string
	if pgpKeyOverride := os.Getenv("OCP_SIGNATURE_VERIFICATION_PK"); len(pgpKeyOverride) != 0 {
		log.Debug("OCP_SIGNATURE_VERIFICATION_PK environment variable set: using PGP key in %s for OCP signature verification", pgpKeyOverride)
		pgpKeyOverrideContent, err := os.ReadFile(pgpKeyOverride)
		if err != nil {
			log.Warn("unable to read file %s, fallback to using default PGP key", pgpKeyOverride)
		}
		if len(pgpKeyOverrideContent) > 0 {
			pgp = string(pgpKeyOverrideContent)
		} else {
			pgp = defaultPK
		}
	} else {
		pgp = defaultPK
	}
	return &SignatureSchema{Log: log, Config: config, Opts: opts, pgpKey: pgp}
}

// GenerateReleaseSignatures
func (o SignatureSchema) GenerateReleaseSignatures(ctx context.Context, images []v2alpha1.CopyImageSchema) ([]v2alpha1.CopyImageSchema, error) {
	var imgs []v2alpha1.CopyImageSchema
	// set up http object
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		Proxy:           http.ProxyFromEnvironment,
	}
	httpClient := &http.Client{Transport: tr}

	for _, img := range images {
		var data []byte
		imgSpec, err := image.ParseRef(img.Source)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] parsing image digest")
		}
		digest := imgSpec.Digest

		// OCPBUGS-56009
		// not worrying about code complexity as this module will eventually be deprecated
		// in favor of the cosign signature work
		if o.Opts.Global.IgnoreReleaseSignature && len(o.Config.Mirror.Platform.Release) > 0 {
			imgs = append(imgs, img)
			continue
		}
		// nolint: nestif
		if digest != "" {
			o.Log.Debug("signature digest %s", digest)
			// check if the image is in the cache else
			// do a lookup and download it to cache
			sigFiles, err := os.ReadDir(o.Opts.Global.WorkingDir + SignatureDir)
			if err != nil {
				o.Log.Debug("[GenerateReleaseSignatures] no directory found for signatures %w", err)
			}
			for _, file := range sigFiles {
				if strings.Contains(file.Name(), digest) {
					data, err = os.ReadFile(o.Opts.Global.WorkingDir + SignatureDir + file.Name())
					if err != nil {
						o.Log.Warn("[GenerateReleaseSignatures] could not read %s %w", file.Name(), err)
					}
					break
				}
			}
		}

		// we dont have the current digest in cache
		// nolint: nestif
		if len(data) == 0 {
			signatureURL := defaultSignatureURL
			if signatureURLOvr := os.Getenv("OCP_SIGNATURE_URL"); len(signatureURLOvr) != 0 {
				if parsedURL, err := url.ParseRequestURI(signatureURLOvr); err != nil {
					o.Log.Debug("Invalid URL provided in OCP_SIGNATURE_URL: %s, falling back to default SignatureURL", signatureURLOvr)
				} else {
					o.Log.Debug("OCP_SIGNATURE_URL environment variable set: using %s as base URL for signature retrieval", signatureURLOvr)
					signatureURL = parsedURL.String()
				}
			}
			req, _ := http.NewRequest("GET", signatureURL+"sha256="+digest+"/signature-1", nil)
			// req.Header.Set("Authorization", "Basic "+generic.Token)
			req.Header.Set(ContentType, ApplicationJson)
			resp, err := httpClient.Do(req)
			if err != nil {
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf("http request %w", err)
			}
			defer func() {
				if resp != nil && resp.Body != nil {
					resp.Body.Close()
				}
			}()
			if resp.StatusCode == http.StatusOK {
				o.Log.Debug("response from signature lookup %d", resp.StatusCode)
				data, err = io.ReadAll(resp.Body)
				if err != nil {
					return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] reading response body %w", err)
				}
			}
		}

		if len(data) == 0 {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] no signature found for %s image %s", digest, img.Source)
		}

		pkBytes := []byte(o.pgpKey)

		keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(pkBytes))
		// keyring, err := openpgp.ReadKeyRing(bytes.NewReader([]byte(pkBytes)))
		if err != nil {
			o.Log.Error("%v", err)
		}
		o.Log.Debug("keyring %v", keyring)

		md, err := openpgp.ReadMessage(bytes.NewReader(data), keyring, nil, nil)
		if err != nil {
			o.Log.Error("%v could not read the message:", err)
		}
		if md == nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] unable to read signature message for %s image %s", digest, img.Source)
		}
		if !md.IsSigned {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] message was not signed for %s image %s", digest, img.Source)
		}
		if md.SignatureError != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] signature error for %s image %s", digest, img.Source)
		}
		if md.SignedBy == nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] invalid signature for %s image %s", digest, img.Source)
		}

		// update the image with the actual reference from the contents json
		signSchema, err := parser.ParseJsonReader[v2alpha1.SignatureContentSchema](md.UnverifiedBody)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] unmarshal json %w", err)
		}
		img.Source = signSchema.Critical.Identity.DockerReference
		o.Log.Debug("image found : %s", signSchema.Critical.Identity.DockerReference)

		o.Log.Trace("field isEncrypted %v", md.IsEncrypted)
		o.Log.Trace("field EencryptedToKeyIds %v", md.EncryptedToKeyIds)
		o.Log.Trace("field IsSymmetricallyEncrypted %v", md.IsSymmetricallyEncrypted)
		o.Log.Trace("field DecryptedWith %v", md.DecryptedWith)
		o.Log.Trace("field IsSigned %v", md.IsSigned)
		o.Log.Trace("field SignedByKeyId %v", md.SignedByKeyId)
		o.Log.Trace("field SignedBy %v", md.SignedBy)
		o.Log.Trace("field LiteralData %v", md.LiteralData)
		o.Log.Trace("field SignatureError %v", md.SignatureError)
		o.Log.Trace("field Signature %v", md.Signature)
		// o.Log.Trace("field SignatureV3 %v", md.SignatureV3.IssuerKeyId)
		// o.Log.Trace("field SignatureV3 %v", md.SignatureV3.CreationTime)

		if md.Signature != nil {
			if md.Signature.SigLifetimeSecs != nil {
				expiry := md.Signature.CreationTime.Add(time.Duration(*md.Signature.SigLifetimeSecs) * time.Second)
				if time.Now().After(expiry) {
					o.Log.Debug("signature expired on %v ", expiry)
				}
			}
		} else if md.SignatureV3 == nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] unexpected openpgp.MessageDetails: neither Signature nor SignatureV3 is set for %s image %s", digest, img.Source)
		}

		// write signature to cache
		newImgSpec, err := image.ParseRef(img.Source)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] could not parse identity docker reference image %w", err)
		}
		sigFilePath := fmt.Sprintf("%s%s/%s-sha256-%s", o.Opts.Global.WorkingDir, SignatureDir, newImgSpec.Tag, digest)
		if _, err := os.Stat(sigFilePath); err != nil {
			ferr := os.WriteFile(sigFilePath, data, 0600)
			if ferr != nil {
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] writing %w", ferr)
			}
		}
		imgs = append(imgs, img)
	}
	return imgs, nil
}
