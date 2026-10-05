FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/gnotifd ./cmd/gnotifd
RUN mkdir /data-dir

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/gnotifd /usr/local/bin/gnotifd
COPY --from=build --chown=nonroot:nonroot /data-dir /data
VOLUME /data
USER nonroot
ENTRYPOINT ["gnotifd"]
CMD ["-db", "/data/gnotif.db", "-listen", "0.0.0.0:8080"]
