FROM golang:1.27-alpine AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/restart-controller main.go

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/restart-controller /usr/local/bin/restart-controller
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/restart-controller"]
