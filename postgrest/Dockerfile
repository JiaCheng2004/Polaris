FROM postgrest/postgrest:latest
ENV TZ=America/Los_Angeles

COPY postgrest.conf /etc/postgrest.conf

# Copy certs if you have them; otherwise, comment out temporarily
# COPY server.crt /etc/ssl/server.crt
# COPY server.key /etc/ssl/server.key
# RUN chmod 600 /etc/ssl/server.key

CMD ["postgrest", "/etc/postgrest.conf"]
