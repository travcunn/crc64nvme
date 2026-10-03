module github.com/travcunn/crc64nvme/bench

go 1.25.0

replace github.com/travcunn/crc64nvme => ../

require (
	github.com/minio/crc64nvme v1.1.1
	github.com/travcunn/crc64nvme v0.0.0-00010101000000-000000000000
)

require (
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
)
