package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// InitResult 包含初始化后的关键信息
type InitResult struct {
	Token       string
	Fingerprint string
	CertPath    string
	KeyPath     string
	ConfigPath  string
}

// Init 生成自签证书、Token，并写入配置文件
func Init(configDir string) (*InitResult, error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	// 生成 ECDSA P256 密钥
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// 收集本机所有 IP 地址
	ips := []net.IP{net.ParseIP("127.0.0.1")}
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip == nil || ip.IsLoopback() {
					continue
				}
				ips = append(ips, ip)
			}
		}
	}

	// 生成自签 X509 证书，10 年有效期
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "reach-agent",
			Organization: []string{"reach"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           ips,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	// 计算证书 SHA256 指纹
	fingerprint := sha256.Sum256(certDER)
	fingerprintHex := hex.EncodeToString(fingerprint[:])

	// 生成 128 位随机 Token (hex encoded, 64 字符)
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// 写 cert.pem
	certPath := filepath.Join(configDir, "cert.pem")
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("create cert.pem: %w", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		certFile.Close()
		return nil, fmt.Errorf("write cert.pem: %w", err)
	}
	certFile.Close()

	// 写 key.pem (0600)
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPath := filepath.Join(configDir, "key.pem")
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("create key.pem: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		keyFile.Close()
		return nil, fmt.Errorf("write key.pem: %w", err)
	}
	keyFile.Close()

	// 写 config.yaml (0600)
	cfg := &AgentConfig{
		Port:       7100,
		Token:      token,
		TLSCert:    certPath,
		TLSKey:     keyPath,
		MaxOutput:  10485760,
		MaxTimeout: 600,
	}
	configPath := filepath.Join(configDir, "config.yaml")
	configFile, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("create config.yaml: %w", err)
	}
	enc := yaml.NewEncoder(configFile)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		configFile.Close()
		return nil, fmt.Errorf("write config.yaml: %w", err)
	}
	configFile.Close()

	return &InitResult{
		Token:       token,
		Fingerprint: fingerprintHex,
		CertPath:    certPath,
		KeyPath:     keyPath,
		ConfigPath:  configPath,
	}, nil
}
