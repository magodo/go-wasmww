module github.com/magodo/go-wasmww

go 1.21.0

require (
	github.com/hack-pad/go-webworkers v0.1.0
	github.com/hack-pad/safejs v0.1.1
	github.com/magodo/chanio v0.0.0-20230901115729-fd5b8225ff0b
)

require github.com/pkg/errors v0.9.1 // indirect

// Use the upstream one after https://github.com/hack-pad/go-webworkers/pull/6 is merged
replace github.com/hack-pad/go-webworkers => ../go-webworkers
