module github.com/magodo/go-wasmww

go 1.21.0

require (
	github.com/google/uuid v1.3.1
	github.com/hack-pad/go-webworkers v0.1.0
	github.com/hack-pad/safejs v0.1.1
	github.com/magodo/chanio v0.0.0-20230905063744-5f1bf45eacbc
)

require github.com/pkg/errors v0.9.1 // indirect

// Use the upstream one after https://github.com/hack-pad/go-webworkers/pull/6 is merged
replace github.com/hack-pad/go-webworkers => ../go-webworkers
