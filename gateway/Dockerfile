FROM envoyproxy/envoy:v1.33-latest
ARG APM_TYPE
COPY --chmod=755 ./envoy.${APM_TYPE}.yaml /etc/envoy/envoy.yaml
