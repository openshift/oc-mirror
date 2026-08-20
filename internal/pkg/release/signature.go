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
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"

	// nolint
	// NOTE: Do NOT replace this with the community-maintained fork
	// github.com/ProtonMail/go-crypto/openpgp: that library intentionally dropped
	// support for verifying legacy PGP V3 signature packets (MessageDetails no
	// longer exposes SignatureV3, and V3-signed messages always fail with a
	// SignatureError). OpenShift release payloads signed before ~August 2023
	// (roughly OCP 4.1 through 4.13.10) use V3 signatures, and those releases are
	// still served by the Cincinnati graph and still pullable from quay.io today,
	// so oc-mirror must keep verifying them. Migrating to ProtonMail/go-crypto
	// would silently break signature verification for any of those older
	// releases/channels. Revisit only once V3-signed releases are no longer
	// reachable, or if a hybrid verifier is implemented to handle V3 separately.
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
	// OCPBUGS-56009
	if o.Opts.Global.IgnoreReleaseSignature && len(o.Config.Mirror.Platform.Release) > 0 {
		return images, nil
	}

	// set up http object
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		Proxy:           http.ProxyFromEnvironment,
	}
	httpClient := &http.Client{Transport: tr}

	signatureURL := defaultSignatureURL
	if signatureURLOvr := os.Getenv("OCP_SIGNATURE_URL"); len(signatureURLOvr) != 0 {
		if parsedURL, err := url.ParseRequestURI(signatureURLOvr); err != nil {
			o.Log.Error("Invalid URL provided in OCP_SIGNATURE_URL: %s, falling back to default SignatureURL", signatureURLOvr)
		} else {
			o.Log.Debug("OCP_SIGNATURE_URL environment variable set: using %s as base URL for signature retrieval", signatureURLOvr)
			signatureURL = parsedURL.String()
		}
	}

	var imgs []v2alpha1.CopyImageSchema
	for _, img := range images {
		imgSpec, err := image.ParseRef(img.Source)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] parsing image digest")
		}
		digest := imgSpec.Digest

		var data []byte
		if digest != "" {
			o.Log.Debug("signature digest %s", digest)
			// check if the image is in the cache
			if data = o.getCachedSignature(digest); len(data) == 0 {
				// we dont have the current digest in cache, do a lookup and download it
				data, err = o.downloadSignature(ctx, httpClient, signatureURL, digest)
				if err != nil {
					return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] failed to get signature for %s: %w", digest, err)
				}
			}
		}
		if len(data) == 0 {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] no signature found for %s image %s", digest, img.Source)
		}

		src, err := o.verifySignature(imgSpec.Algorithm, digest, data)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] invalid signature for %s image %s: %w", digest, img.Source, err)
		}
		img.Source = src

		// write signature to cache
		if err := o.saveSignature(img.Source, digest, data); err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("[GenerateReleaseSignatures] save signature for %s image %s: %w", digest, img.Source, err)
		}

		// OCPBUGS-52562
		// add a check to ensure there are no duplicates
		// ideally it would be better to add this at the early stages of this process
		// but due to the conversion of the 'Source' field in line 155 we have to do this after the fact
		if !slices.Contains(imgs, img) {
			imgs = append(imgs, img)
		}
	}
	return imgs, nil
}

func (o SignatureSchema) downloadSignature(ctx context.Context, client *http.Client, signatureURL string, digest string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", signatureURL+"sha256="+digest+"/signature-1", nil) //nolint:gosec //FIXME: G704: SSRF via taint analysis
	req.Header.Set(ContentType, ApplicationJson)
	resp, err := client.Do(req) //nolint:gosec //FIXME: G704: SSRF via taint analysis
	if err != nil {
		return nil, fmt.Errorf("http request %w", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	o.Log.Debug("response from signature lookup %d", resp.StatusCode)
	switch resp.StatusCode {
	case http.StatusOK:
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body %w", err)
		}
		return data, nil
	case http.StatusNotFound:
		// NOTE: 404 is a special case so we keep the same error message as before
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected signature request response: %s", resp.Status)
	}
}

func (o SignatureSchema) getCachedSignature(digest string) []byte {
	sigFiles, err := os.ReadDir(filepath.Join(o.Opts.Global.WorkingDir, SignatureDir))
	if err != nil {
		o.Log.Debug("[GenerateReleaseSignatures] no directory found for signatures %v", err)
	}

	var data []byte
	for _, file := range sigFiles {
		if !strings.Contains(file.Name(), digest) {
			continue
		}
		data, err = os.ReadFile(filepath.Join(o.Opts.Global.WorkingDir, SignatureDir, file.Name()))
		if err != nil {
			o.Log.Warn("[GenerateReleaseSignatures] could not read %s %v", file.Name(), err)
		}
		break
	}

	return data
}

//nolint:cyclop // not worrying about code complexity as this module will eventually be deprecated in favor of the cosign signature work
func (o SignatureSchema) verifySignature(algorithm string, digest string, data []byte) (string, error) {
	pkBytes := []byte(o.pgpKey)

	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(pkBytes))
	if err != nil {
		o.Log.Error("%v", err)
	}
	o.Log.Debug("keyring %v", keyring)

	md, err := openpgp.ReadMessage(bytes.NewReader(data), keyring, nil, nil)
	if err != nil {
		return "", fmt.Errorf("unable to read signature message: %w", err)
	}
	if md == nil {
		return "", fmt.Errorf("unable to read signature message")
	}
	if !md.IsSigned {
		return "", fmt.Errorf("message was not signed")
	}
	if md.SignedBy == nil {
		return "", fmt.Errorf("invalid signature")
	}

	// NOTE: md.SignatureError cannot be trusted until md.UnverifiedBody has been
	// read to EOF: per the openpgp API, the signature is verified as the body is
	// consumed, and the check can only complete once the whole message has been
	// read. So we must fully read UnverifiedBody first, then check SignatureError.
	//
	// update the image with the actual reference from the contents json
	signSchema, err := parser.ParseJsonReader[v2alpha1.SignatureContentSchema](md.UnverifiedBody)
	if err != nil {
		return "", fmt.Errorf("unmarshal json %w", err)
	}
	if md.SignatureError != nil {
		return "", fmt.Errorf("signature error: %w", md.SignatureError)
	}
	// Sanity: even if the signature is valid, let's double-check it's the signature of the payload we expect.
	if readDigest := signSchema.Critical.Image.DockerManifestDigest; readDigest != fmt.Sprintf("%s:%s", algorithm, digest) {
		return "", fmt.Errorf("mismatched digest %q: expected %q", readDigest, digest)
	}
	imgSource := signSchema.Critical.Identity.DockerReference
	o.Log.Debug("image found : %s", imgSource)

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
		return "", fmt.Errorf("unexpected openpgp.MessageDetails: neither Signature nor SignatureV3 is set for %s", imgSource)
	}

	return imgSource, nil
}

func (o SignatureSchema) saveSignature(imgRef string, digest string, data []byte) error {
	newImgSpec, err := image.ParseRef(imgRef)
	if err != nil {
		return fmt.Errorf("could not parse identity docker reference image %w", err)
	}
	sigFilePath := filepath.Join(o.Opts.Global.WorkingDir, SignatureDir, fmt.Sprintf("%s-sha256-%s", newImgSpec.Tag, digest))
	//nolint:gosec // FIXME: G703: sanitize paths
	if _, err := os.Stat(sigFilePath); err != nil {
		if err := os.WriteFile(sigFilePath, data, 0o600); err != nil {
			return fmt.Errorf("write signature: %w", err)
		}
	}
	return nil
}
