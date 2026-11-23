package tunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"portman/internal/config"
)

// testPKI is a throwaway certificate authority for tests.
type testPKI struct {
	der  []byte
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func newPKI(t *testing.T) *testPKI {
	t.Helper()
	key := newKey(t)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "portman-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testPKI{der: der, cert: cert, key: key}
}

// issue creates a leaf certificate signed by the CA, for a server or a client.
func (p *testPKI) issue(t *testing.T, cn string, server bool) tls.Certificate {
	t.Helper()
	key := newKey(t)
	usage := x509.ExtKeyUsageClientAuth
	if server {
		usage = x509.ExtKeyUsageServerAuth
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.cert, &key.PublicKey, p.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func encodeCert(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// writeIdentity saves a leaf certificate and its key as PEM files.
func writeIdentity(t *testing.T, dir, name string, c tls.Certificate) (certPath, keyPath string) {
	t.Helper()
	keyDER, err := x509.MarshalECPrivateKey(c.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	certPath = filepath.Join(dir, name+".pem")
	keyPath = filepath.Join(dir, name+".key")
	if err := os.WriteFile(certPath, encodeCert(c.Certificate[0]), 0o600); err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

// roundTrip completes a TLS handshake and sends one byte through the echo server.
func roundTrip(addr string, cfg *tls.Config) error {
	conn, err := tls.Dial("tcp", addr, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("x")); err != nil {
		return err
	}
	buf := make([]byte, 1)
	_, err = io.ReadFull(conn, buf)
	return err
}

func TestMutualTLS(t *testing.T) {
	ca := newPKI(t)
	otherCA := newPKI(t)
	dir := t.TempDir()

	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, encodeCert(ca.der), 0o600); err != nil {
		t.Fatal(err)
	}
	serverCertPath, serverKeyPath := writeIdentity(t, dir, "server", ca.issue(t, "127.0.0.1", true))

	tlsCfg, err := serverTLSConfig(config.Config{
		Mode:         "server",
		ListenAddr:   "127.0.0.1:0",
		CertFile:     serverCertPath,
		KeyFile:      serverKeyPath,
		ClientCAFile: caPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				io.Copy(c, c)
			}()
		}
	}()

	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	validClient := ca.issue(t, "bridge-client", false)
	foreignClient := otherCA.issue(t, "intruder", false)

	cases := []struct {
		name   string
		client []tls.Certificate
		wantOK bool
	}{
		{"valid client certificate", []tls.Certificate{validClient}, true},
		{"no client certificate", nil, false},
		{"certificate from another CA", []tls.Certificate{foreignClient}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clientCfg := &tls.Config{
				RootCAs:      roots,
				ServerName:   "127.0.0.1",
				MinVersion:   tls.VersionTLS12,
				Certificates: tc.client,
			}
			err := roundTrip(ln.Addr().String(), clientCfg)
			if tc.wantOK && err != nil {
				t.Fatalf("expected success, got: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Fatal("expected the server to reject this client")
			}
		})
	}
}
