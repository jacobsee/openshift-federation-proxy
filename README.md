# OpenShift Federation Proxy

## What is this?

OpenShift Container Platform comes with a built-in, OpenShift-managed instance of [Prometheus](https://prometheus.io/) to monitor and alert on important cluster events. Prometheus has a native way of federating (sharing data) with other Prometheus instances, in the event that you have multiple instances that you would like to monitor from a single point. Unfortunately, this Prometheus federation feature cannot be used out-of-the-box with Openshift-managed Prometheus instances, due to the fact that OpenShift wraps its Prometheus instance with [oauth-proxy](https://github.com/openshift/oauth-proxy) for security reasons, which Prometheus cannot connect with.

This project aims to enable you to monitor the health of multiple OpenShift clusters via Prometheus federation in the simplest way possible.

## How does it work?

This project, as its name implies, proxies connections to the `/federate` endpoint on Prometheus instances protected by OpenShift's `oauth-proxy`. Given an account that can authenticate to the target cluster, it negotiates OAuth, and presents a "normal" `/federate` endpoint for Prometheus instances to connect with.

```
+--------------------------+                        +--------------------+
|                          |                        |                    |
|      Monitored           |                        |     Monitoring     |
|      OpenShift           |     Does Not Work      |     Prometheus     |
|      Cluster             |                        |     Instance       |
|                  /federate  <-------------------- |                    |
|                  (secure)|                        |                    |
+--------------------------+                        +--------------------+





+--------------------------+           +-----------------+           +--------------------+
|                          |           |                 |           |                    |
|      Monitored           |           |  OpenShift      |           |     Monitoring     |
|      OpenShift           |           |  Federation     |           |     Prometheus     |
|      Cluster             |           |  Proxy          |           |     Instance       |
|                  /federate  <------- |         /federate <-------- |                    |
|                  (secure)|           |                 |           |                    |
+--------------------------+           +-----------------+           +--------------------+
```

## What does setup look like?

Add an entry to `scrape_configs` in your `prometheus.yml` file:

```yaml
scrape_configs:
  - job_name: 'openshift'
    scrape_interval: 30s
    params:
      'match[]':
        - 'kubelet_volume_stats_available_bytes' # Any queries that you'd like to federate should be defined here
    file_sd_configs:
      - files:
        - /etc/prometheus/targets/openshift_targets/*.yml # Any targets you'd like to monitor should be defined in target files here
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_endpoint
      - source_labels: [__param_endpoint]
        target_label: instance
      - target_label: __address__
        replacement: 127.0.0.1:8080 # This assumes that the OpenShift Federation Proxy is running on the same machine as Prometheus itself.
```

Any number of target files in `/etc/prometheus/targets/openshift_targets/` can then look like:

```yaml
- targets:
  - my-cluster-url.my-domain.com
  labels:
    "__param_username": "monitoring_service_account"
    "__param_password": "monitoring_password"
```

## Alternatives

You should also be aware of the [Thanos](https://thanos.io/) project, which is [documented for use with OpenShift here](https://www.openshift.com/blog/federated-prometheus-with-thanos-receive). By comparison, Thanos requires some components to run on the target OpenShift clusters, and stores metrics in some external persistent storage service, like S3. This project requires no modification to the target clusters and is generally aimed at use-cases where a simpler architecture is desired.
