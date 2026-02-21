FROM gcr.io/distroless/static:nonroot
COPY orb /orb
ENTRYPOINT ["/orb"]
