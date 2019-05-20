FROM alpine as builder
RUN mkdir /opt/tmp
RUN chmod 1777 /opt/tmp
FROM scratch
COPY --from=builder /opt/tmp /tmp
COPY ./metallb-bounce .
ENTRYPOINT ["./metallb-bounce"]
