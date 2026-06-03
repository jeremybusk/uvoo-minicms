.PHONY: dev build package package-linux run web docker-up docker-build docker-down

web:
	cd web && npm install && npm run build

build:
	bash scripts/build.sh

package:
	bash scripts/package.sh

package-linux:
	bash scripts/package-linux.sh

run: build
	./bin/uvoo-minicms

dev:
	mkdir -p data/uploads
	( cd web && npm install && npm run dev ) & go run ./cmd/uvoo-minicms

docker-build:
	docker compose build uvoo-minicms

docker-up:
	docker compose up -d --build --remove-orphans uvoo-minicms

docker-down:
	docker compose down
