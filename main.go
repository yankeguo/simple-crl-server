package main

import (
	"bufio"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cacheDuration = 1 * time.Hour
)

var (
	cacheDir = filepath.Join(".", "temp")
	tlsDir   = filepath.Join(".", "tls")
	confDir  = filepath.Join(".", "conf")
)

type RevokedCert struct {
	SerialNumber   *big.Int
	RevocationTime time.Time
	Reason         int
}

type CRLServer struct {
	crtFile  string
	keyFile  string
	listFile string

	mu          sync.RWMutex
	cachedCRL   []byte
	cacheTime   time.Time
	cacheNumber *big.Int
}

func main() {
	// Ensure temp directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Fatalf("Failed to create cache directory: %v", err)
	}

	// Define file paths
	crtFile := filepath.Join(tlsDir, "tls.crt")
	keyFile := filepath.Join(tlsDir, "tls.key")
	listFile := filepath.Join(confDir, "list.txt")

	// Validate certificate and key can be loaded (initial check)
	_, _, err := loadCertAndKey(crtFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to load certificate and key: %v", err)
	}
	log.Println("Certificate and key validated successfully")

	server := &CRLServer{
		crtFile:  crtFile,
		keyFile:  keyFile,
		listFile: listFile,
	}

	// Try to load cached CRL
	server.loadCachedCRL()

	// Set up HTTP handler
	http.HandleFunc("/", server.handleCRL)

	log.Println("Starting CRL server on :8080")
	log.Println("Hot-reload enabled: certificates and revocation list will be reloaded on each CRL generation")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func (s *CRLServer) handleCRL(w http.ResponseWriter, r *http.Request) {
	crl, err := s.getCRL()
	if err != nil {
		log.Printf("Error generating CRL: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pkix-crl")
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", int(cacheDuration.Seconds())))
	w.WriteHeader(http.StatusOK)
	w.Write(crl)
}

func (s *CRLServer) getCRL() ([]byte, error) {
	s.mu.RLock()
	// Check if cache is still valid
	if s.cachedCRL != nil && time.Since(s.cacheTime) < cacheDuration {
		defer s.mu.RUnlock()
		return s.cachedCRL, nil
	}
	s.mu.RUnlock()

	// Need to generate new CRL
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if s.cachedCRL != nil && time.Since(s.cacheTime) < cacheDuration {
		return s.cachedCRL, nil
	}

	// Load certificate and key (hot-reload for Kubernetes Secret updates)
	log.Println("Loading certificate and key...")
	caCert, caPrivKey, err := loadCertAndKey(s.crtFile, s.keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading certificate and key: %w", err)
	}

	// Load revoked certificates (hot-reload for ConfigMap/Secret updates)
	log.Println("Loading revocation list...")
	revokedCerts, err := s.loadRevokedCertificates()
	if err != nil {
		return nil, fmt.Errorf("loading revoked certificates: %w", err)
	}
	log.Printf("Loaded %d revoked certificate(s)", len(revokedCerts))

	// Create CRL template
	now := time.Now()
	crlNumber := big.NewInt(now.Unix())

	template := &x509.RevocationList{
		Number:     crlNumber,
		ThisUpdate: now,
		NextUpdate: now.Add(cacheDuration),
	}

	// Use RevokedCertificateEntries (new API)
	for _, rc := range revokedCerts {
		template.RevokedCertificateEntries = append(template.RevokedCertificateEntries, x509.RevocationListEntry{
			SerialNumber:   rc.SerialNumber,
			RevocationTime: rc.RevocationTime,
			ReasonCode:     rc.Reason,
		})
	}

	// Generate CRL
	crlBytes, err := x509.CreateRevocationList(rand.Reader, template, caCert, caPrivKey)
	if err != nil {
		return nil, fmt.Errorf("creating CRL: %w", err)
	}

	// Update cache
	s.cachedCRL = crlBytes
	s.cacheTime = now
	s.cacheNumber = crlNumber

	// Save to disk
	if err := s.saveCachedCRL(); err != nil {
		log.Printf("Warning: failed to save CRL to cache: %v", err)
	}

	log.Printf("Generated new CRL with number %s", crlNumber.String())
	return crlBytes, nil
}

func (s *CRLServer) loadRevokedCertificates() ([]RevokedCert, error) {
	file, err := os.Open(s.listFile)
	if err != nil {
		// If file doesn't exist, return empty list
		if os.IsNotExist(err) {
			return []RevokedCert{}, nil
		}
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	var revokedCerts []RevokedCert
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse format: [serial_number]:[epoch]:[reason]
		parts := strings.Split(line, ":")
		if len(parts) != 3 {
			log.Printf("Warning: invalid format at line %d: %s", lineNum, line)
			continue
		}

		// Parse serial number (hex)
		serial := new(big.Int)
		if _, ok := serial.SetString(parts[0], 16); !ok {
			log.Printf("Warning: invalid serial number at line %d: %s", lineNum, parts[0])
			continue
		}

		// Parse epoch timestamp
		epoch, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			log.Printf("Warning: invalid epoch at line %d: %s", lineNum, parts[1])
			continue
		}
		revocationTime := time.Unix(epoch, 0)

		// Parse reason code
		reason, err := strconv.Atoi(parts[2])
		if err != nil {
			log.Printf("Warning: invalid reason code at line %d: %s", lineNum, parts[2])
			continue
		}

		revokedCerts = append(revokedCerts, RevokedCert{
			SerialNumber:   serial,
			RevocationTime: revocationTime,
			Reason:         reason,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return revokedCerts, nil
}

func (s *CRLServer) saveCachedCRL() error {
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("crl-%s.der", s.cacheNumber.String()))
	if err := os.WriteFile(cacheFile, s.cachedCRL, 0644); err != nil {
		return err
	}

	// Also save metadata
	metaFile := filepath.Join(cacheDir, fmt.Sprintf("crl-%s.meta", s.cacheNumber.String()))
	metaContent := fmt.Sprintf("%d\n", s.cacheTime.Unix())
	if err := os.WriteFile(metaFile, []byte(metaContent), 0644); err != nil {
		return err
	}

	return nil
}

func (s *CRLServer) loadCachedCRL() {
	// Find the latest cached CRL
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}

	var latestNumber *big.Int
	var latestTime time.Time
	var latestFile string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "crl-") || !strings.HasSuffix(name, ".der") {
			continue
		}

		// Extract number from filename
		numberStr := strings.TrimPrefix(name, "crl-")
		numberStr = strings.TrimSuffix(numberStr, ".der")
		number := new(big.Int)
		if _, ok := number.SetString(numberStr, 10); !ok {
			continue
		}

		// Check if it's the latest
		if latestNumber == nil || number.Cmp(latestNumber) > 0 {
			// Try to read metadata
			metaFile := filepath.Join(cacheDir, fmt.Sprintf("crl-%s.meta", numberStr))
			metaContent, err := os.ReadFile(metaFile)
			if err != nil {
				continue
			}

			timestamp, err := strconv.ParseInt(strings.TrimSpace(string(metaContent)), 10, 64)
			if err != nil {
				continue
			}

			cacheTime := time.Unix(timestamp, 0)

			// Check if cache is still valid
			if time.Since(cacheTime) < cacheDuration {
				latestNumber = number
				latestTime = cacheTime
				latestFile = filepath.Join(cacheDir, name)
			}
		}
	}

	if latestFile != "" {
		crlBytes, err := os.ReadFile(latestFile)
		if err == nil {
			s.mu.Lock()
			s.cachedCRL = crlBytes
			s.cacheTime = latestTime
			s.cacheNumber = latestNumber
			s.mu.Unlock()
			log.Printf("Loaded cached CRL with number %s", latestNumber.String())
		}
	}
}

func loadCertAndKey(crtFile, keyFile string) (*x509.Certificate, crypto.Signer, error) {
	// Load certificate
	certData, err := os.ReadFile(crtFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading cert file: %w", err)
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to parse certificate PEM")
	}

	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing certificate: %w", err)
	}

	// Load private key
	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading key file: %w", err)
	}

	block, _ = pem.Decode(keyData)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to parse key PEM")
	}

	var caPrivKey interface{}

	// Try PKCS8 first
	caPrivKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 RSA
		caPrivKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			// Try EC
			caPrivKey, err = x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				return nil, nil, fmt.Errorf("parsing key: %w", err)
			}
		}
	}

	signer, ok := caPrivKey.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("key does not implement crypto.Signer")
	}

	return caCert, signer, nil
}
