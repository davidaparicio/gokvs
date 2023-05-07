#https://medium.com/@lizrice/non-privileged-containers-based-on-the-scratch-image-a80105d6d341
FROM ubuntu:latest AS userland
RUN useradd --uid 10001 scratchuser

FROM scratch
#ENV PATH=/bin:/usr/local/bin/
COPY --from=userland /etc/passwd /etc/passwd
COPY --chown=scratchuser gokvs /usr/local/bin/gokvs
USER scratchuser
#WORKDIR /usr/local/bin/ # dist/${BuildID}_${BuildTarget}
ENTRYPOINT [ "/usr/local/bin/gokvs" ]