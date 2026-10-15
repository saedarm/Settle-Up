FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /settleup .

FROM gcr.io/distroless/static-debian12
COPY --from=build /settleup /settleup
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/settleup"]
