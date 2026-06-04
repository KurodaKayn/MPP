package contentpipeline

import (
	"net"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	HostEnv     = "CONTENT_PIPELINE_HOST"
	PortEnv     = "CONTENT_PIPELINE_PORT"
	DefaultHost = "content-pipeline-service"
	DefaultPort = "50051"
)

func Addr() string {
	host := strings.TrimSpace(os.Getenv(HostEnv))
	if host == "" {
		host = DefaultHost
	}

	port := strings.TrimSpace(os.Getenv(PortEnv))
	if port == "" {
		port = DefaultPort
	}

	return net.JoinHostPort(host, port)
}

func Dial() (*grpc.ClientConn, error) {
	return grpc.NewClient(Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
}
