FROM golang:1.25-alpine AS build
WORKDIR /src
COPY main.go .
RUN go mod init oauth-mock-server && \
	CGO_ENABLED=0 go build -ldflags="-s -w" -o /oauth-mock-server .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=build /oauth-mock-server /oauth-mock-server
EXPOSE 8080
ENTRYPOINT ["/oauth-mock-server"]
