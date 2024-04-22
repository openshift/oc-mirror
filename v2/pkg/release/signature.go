package release

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"

	// nolint
	"golang.org/x/crypto/openpgp"
)

type SignatureSchema struct {
	Log    clog.PluggableLoggerInterface
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
	pgpKey string
}

func NewSignatureClient(log clog.PluggableLoggerInterface, config v1alpha2.ImageSetConfiguration, opts mirror.CopyOptions) SignatureInterface {
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
func (o SignatureSchema) GenerateReleaseSignatures(ctx context.Context, images []v1alpha3.CopyImageSchema) ([]v1alpha3.CopyImageSchema, error) {

	var data []byte
	var imgs []v1alpha3.CopyImageSchema
	var digest string
	// set up http object
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	httpClient := &http.Client{Transport: tr}

	for _, img := range images {
		imgSpec, err := image.ParseRef(img.Source)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("parsing image digest")
		}
		digest = imgSpec.Digest

		if digest != "" {
			o.Log.Debug("signature %s", digest)
			// check if the image is in the cache else
			// do a lookup and download it to cache
			data, err = os.ReadFile(o.Opts.Global.WorkingDir + SignatureDir + digest)
			if err != nil {
				if os.IsNotExist(err) {
					o.Log.Debug("signature for %s not in cache", digest)
				}
			}
		} else {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("parsing image digest")
		}

		// we have the current digest in cache
		if len(data) == 0 {
			req, _ := http.NewRequest("GET", SignatureURL+"sha256="+digest+"/signature-1", nil)
			//req.Header.Set("Authorization", "Basic "+generic.Token)
			req.Header.Set(ContentType, ApplicationJson)
			resp, err := httpClient.Do(req)
			if err != nil {
				o.Log.Error("http request %v", err)
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
					o.Log.Error("%v", err)
				}
			}
		}

		if len(data) > 0 {
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
			if !md.IsSigned {
				o.Log.Error("not signed")
			}
			content, err := io.ReadAll(md.UnverifiedBody)
			if err != nil {
				o.Log.Error("%v", err)
			}
			if md.SignatureError != nil {
				o.Log.Error("signature error:", md.SignatureError)
			}
			if md.SignedBy == nil {
				o.Log.Error("invalid signature")
			}

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
				o.Log.Error("unexpected openpgp.MessageDetails: neither Signature nor SignatureV3 is set")
			}

			o.Log.Debug("content %s", string(content))
			// update the image with the actaul reference from the contents json
			var signSchema *v1alpha3.SignatureContentSchema
			err = json.Unmarshal(content, &signSchema)
			if err != nil {
				o.Log.Error("could not unmarshal json %v", err)
				return []v1alpha3.CopyImageSchema{}, err
			}
			img.Source = signSchema.Critical.Identity.DockerReference
			o.Log.Debug("image found : %s", signSchema.Critical.Identity.DockerReference)
			// o.Log.Info("public Key : %s", strings.ToUpper(fmt.Sprintf("%x", md.SignedBy.PublicKey.Fingerprint)))

			// write signature to cache
			ferr := os.WriteFile(o.Opts.Global.WorkingDir+SignatureDir+digest, data, 0644)
			if ferr != nil {
				o.Log.Error("%v", ferr)
			}
			imgs = append(imgs, img)
		} else {
			o.Log.Warn("no signature found for %s", digest)
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("no signature found for %s image %s", digest, img.Source)
		}
	}
	return imgs, nil
}
