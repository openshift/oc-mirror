package registry

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// selfSignedTLS holds the files of a CA and a server certificate signed by it,
// valid for localhost, the loopback addresses, and extra host names.
type selfSignedTLS struct {
	caCertPath string
	certPath   string
	keyPath    string
	caPool     *x509.CertPool
}

// generateSelfSignedTLS creates a CA and a server certificate signed by it in dir.
func generateSelfSignedTLS(dir string, extraHosts []string) (*selfSignedTLS, error) {
	notBefore := time.Now().Add(-time.Hour)
	notAfter := time.Now().Add(24 * time.Hour)

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating CA key: %w", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "oc-mirror integration tests CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("creating CA certificate: %w", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, fmt.Errorf("parsing CA certificate: %w", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating server key: %w", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     append([]string{"localhost"}, extraHosts...),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("creating server certificate: %w", err)
	}
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, fmt.Errorf("encoding server key: %w", err)
	}

	t := &selfSignedTLS{
		caCertPath: filepath.Join(dir, "ca.crt"),
		certPath:   filepath.Join(dir, "tls.crt"),
		keyPath:    filepath.Join(dir, "tls.key"),
		caPool:     x509.NewCertPool(),
	}
	t.caPool.AddCert(caCert)

	for path, block := range map[string]*pem.Block{
		t.caCertPath: {Type: "CERTIFICATE", Bytes: caDER},
		t.certPath:   {Type: "CERTIFICATE", Bytes: serverDER},
		t.keyPath:    {Type: "EC PRIVATE KEY", Bytes: serverKeyDER},
	} {
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
			return nil, fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return t, nil
}
