FROM gcr.io/distroless/static:nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/orb /orb
ENTRYPOINT ["/orb"]
