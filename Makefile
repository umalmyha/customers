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

swagger-gen:
	@echo starting to generate swagger docs...
	swag init --parseDependency true
	@echo swagger docs generation finished

proto-gen:
	@echo starting to generate code for gRPC...
	protoc -Iproto --go_out=. --go_opt=module=github.com/umalmyha/customers --go-grpc_out=. --go-grpc_opt=module=github.com/umalmyha/customers --validate_out=paths=source_relative,lang=go:./proto ./proto/*.proto
	@echo gRPC code has been generated

test:
	@echo starting to run tests...
	go test ./internal/repository -v -cover
	@echo test execution completed


