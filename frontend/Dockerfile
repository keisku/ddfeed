FROM nginx:1.27.3 AS dd
RUN apt-get update -y
COPY ./default.dd.conf /etc/nginx/conf.d/default.conf
ADD https://rum-auto-instrumentation.s3.amazonaws.com/installer/latest/install-proxy-datadog.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/install-proxy-datadog.sh
COPY ./dd.sh /usr/local/bin/dd.sh
COPY ./html /usr/share/nginx/html
CMD ["/usr/local/bin/dd.sh"]

FROM nginx:1.27-otel AS otel
RUN apt-get update -y
COPY ./nginx.otel.conf /etc/nginx/nginx.conf
COPY ./default.otel.conf /etc/nginx/conf.d/default.conf
COPY ./html /usr/share/nginx/html
