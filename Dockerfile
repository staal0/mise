FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /mise .
# scratch has no shell, so create the recipe folder here.
RUN mkdir /recipes

FROM scratch
# For HTTPS when importing recipes from websites.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /mise /mise
COPY --from=build --chown=65532:65532 /recipes /recipes
ENV MISE_DIR=/recipes MISE_ADDR=:8080
USER 65532:65532
VOLUME /recipes
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/mise", "healthcheck"]
ENTRYPOINT ["/mise"]
CMD ["serve"]
