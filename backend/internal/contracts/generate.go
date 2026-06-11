package contracts

//go:generate sh -c "set -eu; tmp=$$(mktemp); ruby ../../../contracts/bundle_openapi.rb ../../../contracts/views/backend.openapi.yaml > \"$$tmp\" && go tool oapi-codegen -generate types,skip-prune -package contracts -o openapi.gen.go \"$$tmp\"; rm -f \"$$tmp\""
