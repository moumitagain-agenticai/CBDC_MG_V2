module github.com/fineract/cacti-bridge

go 1.21

require (
	github.com/go-chi/chi/v5 v5.0.11
	github.com/go-chi/cors v1.2.1
	github.com/go-chi/httprate v0.8.0
	github.com/google/uuid v1.5.0
	github.com/hashicorp/go-retryablehttp v0.7.5
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	github.com/prometheus/client_golang v1.18.0
	github.com/sony/gobreaker v0.5.0
	github.com/stretchr/testify v1.8.4
	go.uber.org/zap v1.26.0
	gopkg.in/yaml.v3 v3.0.1
)

// ---------------------------------------------------------------------------
// This bridge is a standalone service and needs no cross-module import. The
// block is left as a placeholder for shared libraries if introduced later.
// ---------------------------------------------------------------------------

require github.com/sirupsen/logrus v1.9.3
