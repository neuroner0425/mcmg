package config

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

// EnsureCertificate verifies that the TLS certificate and private key exist.
// If autoCert is true and files are missing, it generates a valid self-signed ECDSA certificate for localhost.
func EnsureCertificate(certFile, keyFile string, autoCert bool) error {
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return nil // Both files exist and are ready
		}
	}

	if !autoCert {
		return fmt.Errorf("TLS certificate not found and auto_cert is disabled")
	}

	if err := os.MkdirAll(filepath.Dir(certFile), 0755); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0755); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate ECDSA private key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"Minecraft Server Manager Sanctum"},
			CommonName:   "localhost",
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("0.0.0.0")},
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("failed to create x509 certificate: %w", err)
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to encode cert PEM: %w", err)
	}

	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("failed to marshal EC private key: %w", err)
	}

	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open key file: %w", err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}); err != nil {
		return fmt.Errorf("failed to encode key PEM: %w", err)
	}

	return nil
}
