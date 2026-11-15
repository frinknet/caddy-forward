# ---------------------------------------------------------------------------
# Stage 1: Build static caddy binary via xcaddy
# ---------------------------------------------------------------------------
FROM caddy:2-builder-alpine AS build

WORKDIR /build

# Copy the plugin source
COPY . /src/caddy-forward

# Build caddy with the forward plugin
RUN CGO_ENABLED=0 GOOS=linux \
    xcaddy build \
    --with github.com/you/caddy-forward \
    --replace github.com/you/caddy-forward=/src/caddy-forward \
    --output /out/caddy

# ---------------------------------------------------------------------------
# Stage 2: Assets — toybox, certs, tzdata, empty FHS dirs
# ---------------------------------------------------------------------------
RUN apk add --no-cache ca-certificates tzdata curl

ARG TARGETARCH
RUN case "${TARGETARCH}" in \
      "amd64") TOYBOX_ARCH="x86_64" ;; \
      "arm64") TOYBOX_ARCH="aarch64" ;; \
      *) echo "Unsupported arch: ${TARGETARCH}" && exit 1 ;; \
    esac && \
    mkdir -p \
        /out/bin \
        /out/etc/caddy \
        /out/var/lib/caddy \
        /out/var/cache/caddy && \
    curl -fsSL "https://landley.net/toybox/bin/toybox-${TOYBOX_ARCH}" \
        -o /out/toybox && \
    chmod +x /out/toybox

RUN mkdir -p /out/licenses/caddy /out/licenses/toybox \
    && cp /go/pkg/mod/github.com/caddyserver/caddy/v2@*/LICENSE /out/licenses/caddy/LICENSE \
    && curl -fsSL "https://codeberg.org/landley/toybox/raw/branch/master/LICENSE" -o /out/licenses/toybox/LICENSE \
    && find /go/pkg/mod/github.com -maxdepth 3 -iname "LICENSE*" -exec sh -c 'pkg=$(echo "$1" | cut -d/ -f5-6 | tr / _); mkdir -p "/out/licenses/plugins/$pkg" && cp "$1" "/out/licenses/plugins/$pkg/"' _ {} \;

# ---------------------------------------------------------------------------
# Stage 3: Compress binaries for absolute smallest image
# ---------------------------------------------------------------------------
RUN apk add --no-cache upx \
    && upx --best --lzma /out/toybox \
    && upx --best --lzma /out/caddy

# ---------------------------------------------------------------------------
# Stage 4: Final image shrunk and ready to rock
# ---------------------------------------------------------------------------
FROM scratch

COPY --from=build /out/licenses            /usr/share/licenses
COPY --from=build /etc/ssl/certs           /etc/ssl/certs
COPY --from=build /usr/share/zoneinfo      /usr/share/zoneinfo
COPY --from=build /out/etc/caddy           /etc/caddy
COPY --from=build /out/var/lib/caddy       /var/lib/caddy
COPY --from=build /out/var/cache/caddy     /var/cache/caddy
COPY --from=build /out/toybox              /bin/toybox
COPY --from=build /out/caddy               /bin/caddy
COPY             LICENSE                   /usr/share/licenses/caddy/LICENSE

RUN ["/bin/toybox", "sh", "-c", "cd /bin && for i in $(./toybox); do if [ \"$i\" != toybox ]; then ./toybox ln -s toybox \"$i\"; fi; done && test -L /bin/sh && test -L /bin/ls"]
RUN mkdir -p /tmp && chmod 1777 /tmp

VOLUME ["/var/lib/caddy"]

ENV PATH="/bin:/usr/bin:/usr/local/bin"
ENV XDG_CONFIG_HOME="/etc/caddy"
ENV XDG_DATA_HOME="/var/lib/caddy"
ENV XDG_CACHE_HOME="/var/cache/caddy"
ENV TMPDIR="/tmp"
ENV HOME="/tmp"

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD /bin/toybox wget -O /dev/null -T 2 http://127.0.0.1:2019/config/ >/dev/null 2>&1

EXPOSE 80 443 443/udp

WORKDIR /srv/www
ENTRYPOINT ["/bin/caddy"]
CMD ["run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
