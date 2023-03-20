##################################
# STEP 1 build executable binary
##################################
# First stage: build the executable.

ARG MODULE_NAME=app
ARG APP_NAME=app

FROM golang:1.20 AS builder
ARG MODULE_NAME
ARG APP_NAME

WORKDIR /app

COPY ./GIT_COMMIT ./GIT_COMMIT
COPY ./${MODULE_NAME} ./${MODULE_NAME}

WORKDIR /app/${MODULE_NAME}

# Handle the static binding, building and packaging into one static library
RUN  go build -a -ldflags \
"-X common.gitCommit=$(cat /app/GIT_COMMIT) -linkmode external -extldflags '-static' -s -w" \
-o /app/bin/${APP_NAME} /app/${MODULE_NAME}/cmd/${APP_NAME}/main.go

############################
# STEP 2 build a small image
############################
# Use scratch image for minimal image size, or other base image if need more programs in the image
FROM scratch
ARG MODULE_NAME
ARG APP_NAME

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# Copy our static executable.
COPY --from=builder /app/bin/${APP_NAME} /
COPY --from=builder /app/${MODULE_NAME}/configs /configs

ENTRYPOINT ["/app", "--config", "/configs/app.yaml"]
