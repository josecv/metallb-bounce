FROM scratch
COPY ./metallb-bounce .
ENTRYPOINT ["./metallb-bounce"]
