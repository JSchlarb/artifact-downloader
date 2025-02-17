# Artifact Downloader

Artifact Downloader is a small Go application that downloads release artifacts from a GitHub repository. It checks
periodically if a new version is available and downloads the file only when needed.

## Features

- **Periodic Checks:** Downloads files only if there is a new version.
- **Environment Variables:** Configuration is done using environment variables.
- **Graceful Shutdown:** Handles SIGINT and SIGTERM signals to exit cleanly.
- **Kubernetes Ready:** Ideal for running as a sidecar container.

## Environment Variables

- **GITHUB_OWNER** (required):  
  The owner of the GitHub repository.  
  Example: `Skiddle-ID`

- **GITHUB_REPOSITORY** (required):  
  The name of the GitHub repository.  
  Example: `geoip2-mirror`

- **GITHUB_ARTEFACTS** (required):  
  A comma-separated list of artifact names to download.  
  Example: `"GeoLite2-ASN.mmdb,GeoLite2-City.mmdb"`

- **DOWNLOAD_PATH** (required):  
  The local folder path where the files will be saved.  
  Example: `/tmp`

- **CHECK_INTERVAL** (optional):  
  The time interval between checks (e.g., `1h` for one hour).  
  If set to `0` or not set, the program will run once and then exit.

## Example Usage in Kubernetes

Below is an example of how to use Artifact Downloader as a sidecar container in an NGINX Ingress Controller deployment:

```yaml
ingress-nginx:
  controller:
    extraContainers:
      - name: maxmind
        image: ghcr.io/jschlarb/artifact-downloader/downloader:0.0.1
        env:
          - name: GITHUB_OWNER
            value: Skiddle-ID
          - name: GITHUB_REPOSITORY
            value: geoip2-mirror
          - name: GITHUB_ARTEFACTS
            value: "GeoLite2-ASN.mmdb,GeoLite2-City.mmdb"
          - name: DOWNLOAD_PATH
            value: /etc/ingress-controller/geoip/
          - name: CHECK_INTERVAL
            value: 1h
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
              - ALL
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 101
          seccompProfile:
            type: RuntimeDefault
        volumeMounts:
          - name: maxmind
            mountPath: /etc/ingress-controller/geoip
        resources:
          limits:
            memory: 12Mi
          requests:
            cpu: 10m
            memory: 12Mi
    extraVolumeMounts:
      - name: maxmind
        mountPath: /etc/ingress-controller/geoip/
    extraVolumes:
      - name: maxmind
        emptyDir: { }
```

## Building and Running Locally

To build the application, run:

```shell
go build -o artifact-downloader main.go
```

To run the application locally, use:

```shell
GITHUB_OWNER=Skiddle-ID \
GITHUB_REPOSITORY=geoip2-mirror \
GITHUB_ARTEFACTS="GeoLite2-ASN.mmdb,GeoLite2-City.mmdb" \
DOWNLOAD_PATH=/tmp \
CHECK_INTERVAL=0 \
./artifact-downloader
```

## License

This project is licensed under the MIT License.
