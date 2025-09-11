package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"time"
)

func main() {
	_, _, err := SelfSignedForIPs([]string{"192.168.2.2"}, 1000)
	if err != nil {
		return
	}
}

func SelfSignedForIPs(ips []string, days int) (certPEM, keyPEM []byte, err error) {
	// 解析 IP
	var ipSANs []net.IP
	for _, s := range ips {
		if ip := net.ParseIP(s); ip != nil {
			ipSANs = append(ipSANs, ip)
		}
	}
	if len(ipSANs) == 0 {
		return nil, nil, errors.New("no valid IP in list")
	}

	// 私钥（ECDSA P-256）
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// 有效期
	if days <= 0 {
		days = 365
	}
	notBefore := time.Now().Add(-time.Hour)
	notAfter := notBefore.Add(time.Duration(days) * 24 * time.Hour)

	// 序列号
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, err
	}

	// 证书模板（只填 IP 的 SAN，不写 DNSName）
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"Plant Shutter"}},
		NotBefore:    notBefore,
		NotAfter:     notAfter,

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,

		IPAddresses: ipSANs,
	}

	// 自签
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	// 编码 PEM
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
