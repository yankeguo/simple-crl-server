# TLS Certificate and Key

Place your CA certificate and private key in this directory:

- `tls.crt` - CA certificate in PEM format
- `tls.key` - CA private key in PEM format

## Generating Test Certificates

If you need to generate test certificates for development:

### Generate CA Private Key

```bash
# RSA key
openssl genrsa -out tls.key 2048

# Or EC key (recommended)
openssl ecparam -genkey -name prime256v1 -out tls.key
```

### Generate Self-Signed CA Certificate

```bash
# For RSA key
openssl req -new -x509 -key tls.key -out tls.crt -days 3650 \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=Test CA"

# For EC key
openssl req -new -x509 -key tls.key -out tls.crt -days 3650 \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=Test CA"
```

### Verify Certificate

```bash
openssl x509 -in tls.crt -text -noout
```

## Key Format Support

The server supports the following private key formats:
- PKCS#8 (recommended)
- PKCS#1 (RSA keys)
- SEC 1 (EC keys)

All keys must be in PEM format.

