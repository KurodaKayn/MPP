package contracts

//go:generate sh -c "set -eu; trap 'rm -f openapi.bundle.tmp.yaml' EXIT; ruby ../../../contracts/bundle_openapi.rb ../../../contracts/views/backend.openapi.yaml > openapi.bundle.tmp.yaml; go tool oapi-codegen -generate types,skip-prune -package contracts -o openapi.gen.go openapi.bundle.tmp.yaml"
