.PHONY: build run clean test-cert docker-build docker-run

build:
	go build -o simple-crl-server .

run: build
	./simple-crl-server

clean:
	rm -f simple-crl-server
	rm -rf temp/

# Generate test certificate and key
test-cert:
	@echo "Generating test CA certificate and key..."
	@mkdir -p tls
	openssl ecparam -genkey -name prime256v1 -out tls/tls.key
	openssl req -new -x509 -key tls/tls.key -out tls/tls.crt -days 3650 \
		-subj "/C=CN/ST=Test/L=Test/O=Test/CN=Test CA"
	@echo "Certificate and key generated in tls/"
	@echo ""
	@echo "Certificate details:"
	@openssl x509 -in tls/tls.crt -text -noout | head -20

# Create example list.txt
test-list:
	@echo "Creating example list.txt..."
	@mkdir -p conf
	@cp conf/list.txt.example conf/list.txt
	@echo "Example list.txt created at conf/list.txt"

# Setup everything for testing
setup: test-cert test-list
	@echo ""
	@echo "Setup complete! You can now run 'make run' to start the server."

# Docker build
docker-build:
	docker build -t simple-crl-server:latest .

# Docker run
docker-run: docker-build
	docker run -d \
		-p 8080:8080 \
		-v $(PWD)/tls:/app/tls:ro \
		-v $(PWD)/conf:/app/conf:ro \
		-v $(PWD)/temp:/app/temp \
		--name simple-crl-server \
		simple-crl-server:latest

# Stop and remove Docker container
docker-stop:
	docker stop simple-crl-server || true
	docker rm simple-crl-server || true

