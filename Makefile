up:
	@echo Starting containers...
	docker-compose up -d
	@echo Containers are started

up-build:
	@echo Rebuilding images and start containers...
	docker-compose up -d --build
	@echo Containers are started

down:
	@echo Stopping containers...
	docker-compose down
	@echo Containers are stopped

down-v:
	@echo Stopping containers and removing volumes...
	docker-compose down -v
	@echo Containers are stopped

gen_keys:
	@echo "Generating private and public keys..."
	openssl genpkey -algorithm ED25519 -outform pem -out private.ed
	openssl pkey -in private.ed -pubout > public.ed.pub
	@echo "Keys were generated successfully"
