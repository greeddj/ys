# syntax=docker/dockerfile:1
FROM gcr.io/distroless/static-debian13:nonroot
WORKDIR /
COPY ./dist/ys /ys
ENTRYPOINT [ "/ys" ]
