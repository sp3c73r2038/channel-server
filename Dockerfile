FROM alpine
RUN mkdir /app
COPY bin/channel-server /app/
COPY home.html /app/
EXPOSE 7788
CMD ["/app/channel-server", "-addr", ":7788"]
