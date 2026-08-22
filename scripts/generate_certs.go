//go:build ignore
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := os.MkdirAll("certs", 0755); err != nil {
		panic(err)
	}

	// 1. Create CA
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"DBX Inc."},
			Country:       []string{"US"},
			Province:      []string{"CA"},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Golden Gate Bridge"},
			PostalCode:    []string{"94016"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
		AuthorityKeyId:        []byte{1, 2, 3, 4, 5},
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	caBytes, _ := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	
	writePEM("certs/ca.crt", "CERTIFICATE", caBytes)
	writePEM("certs/ca.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(caPrivKey))

	// 2. Create Server Certificate
	serverCert := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization:  []string{"DBX Server"},
		},
		DNSNames:      []string{"localhost", "127.0.0.1", "::1"},
		NotBefore:     time.Now(),
		NotAfter:      time.Now().AddDate(10, 0, 0),
		SubjectKeyId:  []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:      x509.KeyUsageDigitalSignature,
	}

	serverPrivKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	serverBytes, _ := x509.CreateCertificate(rand.Reader, serverCert, ca, &serverPrivKey.PublicKey, caPrivKey)
	writePEM("certs/server.crt", "CERTIFICATE", serverBytes)
	writePEM("certs/server.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverPrivKey))

	// 3. Create Client Certificate
	clientCert := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization:  []string{"DBX Client"},
		},
		NotBefore:     time.Now(),
		NotAfter:      time.Now().AddDate(10, 0, 0),
		ExtKeyUsage:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:      x509.KeyUsageDigitalSignature,
	}
	clientPrivKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	clientBytes, _ := x509.CreateCertificate(rand.Reader, clientCert, ca, &clientPrivKey.PublicKey, caPrivKey)
	writePEM("certs/client.crt", "CERTIFICATE", clientBytes)
	writePEM("certs/client.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(clientPrivKey))

	fmt.Println("Successfully generated CA, Server, and Client certificates in ./certs")
}

func writePEM(filename string, blockType string, bytes []byte) {
	out, err := os.Create(filepath.Clean(filename))
	if err != nil {
		panic(err)
	}
	defer out.Close()
	pem.Encode(out, &pem.Block{Type: blockType, Bytes: bytes})
}



