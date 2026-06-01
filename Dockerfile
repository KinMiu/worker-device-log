# Stage 1: Build binary menggunakan Golang resmi (Gunakan versi Go modern)
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy dependency files terlebih dahulu agar cache optimal
COPY go.mod go.sum ./
RUN go mod download

# Copy seluruh source code proyek
COPY . .

# Build aplikasi Go menjadi binary statis bernama "worker-log"
RUN CGO_ENABLED=0 GOOS=linux go build -o worker-log main.go

# Stage 2: Jalankan binary di image alpine yang super ringan
FROM alpine:3.19

WORKDIR /app

# Copy binary dari stage builder
COPY --from=builder /app/worker-log .

# Copy folder config atau .env jika aplikasi Anda membutuhkannya saat runtime
# (Sesuaikan jika folder config Anda berisi file .json/.yaml yang wajib dibaca)
COPY --from=builder /app/config ./config
COPY --from=builder /app/.env* ./

# Jalankan binary Go
CMD ["./worker-log"]