# frozen_string_literal: true

module KubernetesValidation
  module DataServices
    VALID_REDIS_MAXMEMORY_POLICIES = [
      "noeviction",
      "allkeys-lru",
      "volatile-lru",
      "allkeys-random",
      "volatile-random",
      "volatile-ttl",
      "allkeys-lfu",
      "volatile-lfu",
    ].freeze

    module_function

    def validate_managed(context)
      [["postgres", 5432], ["postgres-reader", 5432], ["redis", 6379]].each do |name, port|
        service = context.require_document("Service", name, "mpp-system")
        next unless service

        context.add_error("managed #{name} Service must be ExternalName") unless service.spec["type"] == "ExternalName"
        service_port = Array(service.spec["ports"]).find { |entry| entry["port"] == port }
        context.add_error("managed #{name} Service must expose port #{port}") unless service_port
        unless service.labels["app.kubernetes.io/managed-by"] == "external-provider"
          context.add_error("managed #{name} Service must be labeled as external-provider managed")
        end
      end

      validate_redis_exporter(context)
    end

    def validate_self_hosted(context)
      [["postgres", 5432], ["postgres-reader", 5432], ["redis", 6379]].each do |name, port|
        service = context.require_document("Service", name, "mpp-system")
        if service
          service_port = Array(service.spec["ports"]).find { |entry| entry["port"] == port }
          context.add_error("self-hosted #{name} Service must expose port #{port}") unless service_port
        end

        next if name == "postgres-reader"

        stateful_set = context.require_document("StatefulSet", name, "mpp-system")
        next unless stateful_set

        pod_spec = stateful_set.pod_spec
        pod_security = pod_spec["securityContext"] || {}
        context.add_error("self-hosted #{name} StatefulSet must not mount service account tokens") unless pod_spec["automountServiceAccountToken"] == false
        context.add_error("self-hosted #{name} StatefulSet must run as non-root") unless pod_security["runAsNonRoot"] == true
        unless pod_security["fsGroupChangePolicy"] == "OnRootMismatch"
          context.add_error("self-hosted #{name} StatefulSet must use OnRootMismatch fsGroup changes")
        end
        unless Array(stateful_set.spec["volumeClaimTemplates"]).any?
          context.add_error("self-hosted #{name} StatefulSet must define persistent storage")
        end

        stateful_set.containers.each do |container|
          validate_container(context, container, "self-hosted #{name} container #{container['name']}")
        end

        validate_self_hosted_redis_persistence(context, stateful_set) if name == "redis"
      end

      validate_self_hosted_read_replica(context)
      validate_self_hosted_backups(context)
      validate_redis_exporter(context)
      validate_self_hosted_network_policy(context, "postgres", 5432, ["backend", "publish-worker", "collab-service", "pgbouncer", "postgres-reader", "postgres-backup"])
      validate_self_hosted_network_policy(context, "postgres-reader", 5432, ["pgbouncer-reader"])
      validate_self_hosted_network_policy(context, "pgbouncer", 5432, ["backend", "publish-worker", "collab-service"])
      validate_self_hosted_network_policy(context, "pgbouncer-reader", 5432, ["backend", "publish-worker", "collab-service"])
      validate_self_hosted_network_policy(context, "redis", 6379, ["backend", "publish-worker", "browser-worker", "collab-service", "redis-exporter", "redis-backup"])
    end

    def validate_redis_ha_nonprod(context)
      validate_redis_ha_services(context)
      validate_redis_ha_config(context)
      validate_redis_ha_stateful_set(
        context,
        name: "redis-ha-primary",
        component: "redis-ha-primary",
        service_name: "redis-ha-primary-headless",
        replicas: 1,
        data_claim: "redis-ha-primary-data",
      )
      validate_redis_ha_stateful_set(
        context,
        name: "redis-ha-replica",
        component: "redis-ha-replica",
        service_name: "redis-ha-replicas-headless",
        replicas: 2,
        data_claim: "redis-ha-replica-data",
      )
      validate_redis_ha_sentinel(context)
      validate_redis_ha_network_policy(context)
      validate_redis_ha_keeps_existing_traffic(context)
    end

    def validate_redis_ha_production(context)
      validate_redis_ha_services(context)
      validate_redis_ha_config(context)
      validate_redis_ha_stateful_set(
        context,
        name: "redis-ha-primary",
        component: "redis-ha-primary",
        service_name: "redis-ha-primary-headless",
        replicas: 1,
        data_claim: "redis-ha-primary-data",
      )
      validate_redis_ha_stateful_set(
        context,
        name: "redis-ha-replica",
        component: "redis-ha-replica",
        service_name: "redis-ha-replicas-headless",
        replicas: 2,
        data_claim: "redis-ha-replica-data",
      )
      validate_redis_ha_sentinel(context)
      validate_redis_ha_network_policy(context)
      validate_redis_ha_production_traffic(context)
    end

    def validate_redis_exporter(context)
      service = context.require_document("Service", "redis-exporter", "mpp-system")
      if service
        service_port = Array(service.spec["ports"]).find { |entry| entry["name"] == "metrics" && entry["port"] == 9121 }
        context.add_error("redis-exporter Service must expose named metrics port 9121") unless service_port
      end

      deployment = context.require_document("Deployment", "redis-exporter", "mpp-system")
      return unless deployment

      pod_spec = deployment.pod_spec
      pod_security = pod_spec["securityContext"] || {}
      context.add_error("redis-exporter Deployment must not mount service account tokens") unless pod_spec["automountServiceAccountToken"] == false
      context.add_error("redis-exporter Deployment must run as non-root") unless pod_security["runAsNonRoot"] == true
      unless pod_security.dig("seccompProfile", "type") == "RuntimeDefault"
        context.add_error("redis-exporter Deployment must use RuntimeDefault seccomp")
      end

      container = deployment.container("redis-exporter")
      return context.add_error("redis-exporter Deployment is missing redis-exporter container") unless container

      context.add_error("redis-exporter container must use oliver006/redis_exporter:v1.86.0") unless container["image"] == "oliver006/redis_exporter:v1.86.0"
      validate_resources(context, container, "redis-exporter container")
      validate_redis_exporter_security(context, container)
      validate_redis_exporter_env(context, container)
      validate_redis_exporter_probes(context, container)
    end

    def validate_redis_ha_services(context)
      {
        "redis-ha-primary" => ["redis-ha-primary", 6379, "redis"],
        "redis-ha-primary-headless" => ["redis-ha-primary", 6379, "redis"],
        "redis-ha-replicas" => ["redis-ha-replica", 6379, "redis"],
        "redis-ha-replicas-headless" => ["redis-ha-replica", 6379, "redis"],
        "redis-ha-sentinel" => ["redis-ha-sentinel", 26379, "sentinel"],
        "redis-ha-sentinel-headless" => ["redis-ha-sentinel", 26379, "sentinel"],
      }.each do |name, (component, port, port_name)|
        service = context.require_document("Service", name, "mpp-system")
        next unless service

        selector = service.spec["selector"] || {}
        unless selector["app.kubernetes.io/component"] == component
          context.add_error("non-prod HA Redis #{name} Service must select #{component} Pods")
        end

        if name.end_with?("-headless") && service.spec["clusterIP"] != "None"
          context.add_error("non-prod HA Redis #{name} Service must be headless")
        end

        service_port = Array(service.spec["ports"]).find do |entry|
          entry["name"] == port_name && entry["port"] == port
        end
        context.add_error("non-prod HA Redis #{name} Service must expose #{port_name} port #{port}") unless service_port
      end
    end

    def validate_redis_ha_config(context)
      config = context.require_document("ConfigMap", "redis-ha-config", "mpp-system")
      return unless config

      redis_config = redis_config_lines(config.data["redis.conf"])
      validate_redis_runtime_common_policy(context, redis_config)
      validate_redis_persistence_common_policy(context, redis_config)
      validate_overlay_redis_persistence_policy(context, redis_config)

      {
        "replica-read-only yes" => "non-prod HA Redis config must keep replicas read-only",
        "repl-diskless-sync yes" => "non-prod HA Redis config must enable diskless replica sync",
      }.each do |line, message|
        context.add_error(message) unless redis_config.include?(line)
      end
    end

    def validate_redis_ha_stateful_set(context, name:, component:, service_name:, replicas:, data_claim:)
      stateful_set = context.require_document("StatefulSet", name, "mpp-system")
      return unless stateful_set

      context.add_error("non-prod HA Redis #{name} StatefulSet must use #{service_name}") unless stateful_set.spec["serviceName"] == service_name
      context.add_error("non-prod HA Redis #{name} StatefulSet must run #{replicas} replicas") unless stateful_set.spec["replicas"] == replicas

      selector_component = stateful_set.spec.dig("selector", "matchLabels", "app.kubernetes.io/component")
      pod_component = stateful_set.pod_labels["app.kubernetes.io/component"]
      unless selector_component == component && pod_component == component
        context.add_error("non-prod HA Redis #{name} StatefulSet must select #{component} Pods")
      end

      validate_redis_ha_pod_security(context, stateful_set, "non-prod HA Redis #{name} StatefulSet")
      validate_redis_ha_scheduling_spread(context, stateful_set, "non-prod HA Redis #{name} StatefulSet")

      redis_container = stateful_set.container("redis")
      unless redis_container
        context.add_error("non-prod HA Redis #{name} StatefulSet must define a redis container")
        return
      end

      context.add_error("non-prod HA Redis #{name} container must use redis:7-alpine") unless redis_container["image"] == "docker.io/library/redis:7-alpine"
      validate_resources(context, redis_container, "non-prod HA Redis #{name} container")
      validate_redis_ha_container_security(context, redis_container, "non-prod HA Redis #{name} container")
      validate_redis_ha_config_mount(context, stateful_set, redis_container, name)
      validate_redis_ha_password_ref(context, redis_container, "non-prod HA Redis #{name} container")
      validate_redis_ping_probe(context, redis_container, "livenessProbe")
      validate_redis_ha_readiness(context, redis_container, name)
      validate_redis_ha_graceful_shutdown(context, stateful_set, redis_container, name)
      validate_redis_ha_data_claim(context, stateful_set, data_claim, name)

      args_text = Array(redis_container["args"]).join("\n")
      if name == "redis-ha-primary"
        unless args_text.include?("--masterauth") && args_text.include?("--requirepass")
          context.add_error("non-prod HA Redis primary must configure auth for clients and replicas")
        end
      else
        unless args_text.include?("--replicaof") && args_text.include?("redis-ha-primary")
          context.add_error("non-prod HA Redis replica must replicate from redis-ha-primary")
        end
      end
    end

    def validate_redis_ha_sentinel(context)
      stateful_set = context.require_document("StatefulSet", "redis-ha-sentinel", "mpp-system")
      return unless stateful_set

      context.add_error("non-prod HA Redis sentinel StatefulSet must use redis-ha-sentinel-headless") unless stateful_set.spec["serviceName"] == "redis-ha-sentinel-headless"
      context.add_error("non-prod HA Redis sentinel StatefulSet must run 3 replicas") unless stateful_set.spec["replicas"] == 3
      context.add_error("non-prod HA Redis sentinel StatefulSet must start Pods in parallel") unless stateful_set.spec["podManagementPolicy"] == "Parallel"

      selector_component = stateful_set.spec.dig("selector", "matchLabels", "app.kubernetes.io/component")
      pod_component = stateful_set.pod_labels["app.kubernetes.io/component"]
      unless selector_component == "redis-ha-sentinel" && pod_component == "redis-ha-sentinel"
        context.add_error("non-prod HA Redis sentinel StatefulSet must select redis-ha-sentinel Pods")
      end

      validate_redis_ha_pod_security(context, stateful_set, "non-prod HA Redis sentinel StatefulSet")
      validate_redis_ha_scheduling_spread(context, stateful_set, "non-prod HA Redis sentinel StatefulSet")

      container = stateful_set.container("sentinel")
      unless container
        context.add_error("non-prod HA Redis sentinel StatefulSet must define a sentinel container")
        return
      end

      context.add_error("non-prod HA Redis sentinel container must use redis:7-alpine") unless container["image"] == "docker.io/library/redis:7-alpine"
      validate_resources(context, container, "non-prod HA Redis sentinel container")
      validate_redis_ha_container_security(context, container, "non-prod HA Redis sentinel container")
      validate_redis_ha_password_ref(context, container, "non-prod HA Redis sentinel container")

      args_text = Array(container["args"]).join("\n")
      {
        "sentinel monitor" => "non-prod HA Redis sentinel must monitor the HA master",
        "redis-ha-primary" => "non-prod HA Redis sentinel must discover redis-ha-primary",
        "sentinel auth-pass" => "non-prod HA Redis sentinel must support REDIS_PASSWORD auth",
        "--sentinel" => "non-prod HA Redis sentinel must start redis-server in sentinel mode",
      }.each do |needle, message|
        context.add_error(message) unless args_text.include?(needle)
      end

      env = Array(container["env"]).each_with_object({}) { |entry, result| result[entry["name"]] = entry["value"] }
      context.add_error("non-prod HA Redis sentinel master name must be mpp-redis-ha") unless env["REDIS_SENTINEL_MASTER_NAME"] == "mpp-redis-ha"
      context.add_error("non-prod HA Redis sentinel quorum must be 2") unless env["REDIS_SENTINEL_QUORUM"] == "2"

      readiness = Array(container.dig("readinessProbe", "exec", "command")).join("\n")
      unless readiness.include?("SENTINEL get-master-addr-by-name") && readiness.include?("SENTINEL ckquorum")
        context.add_error("non-prod HA Redis sentinel readiness must verify master discovery and quorum")
      end

      liveness = Array(container.dig("livenessProbe", "exec", "command")).join("\n")
      unless liveness.include?("redis-cli") && liveness.include?("-p 26379") && liveness.match?(/\bping\b/i)
        context.add_error("non-prod HA Redis sentinel livenessProbe must ping Sentinel")
      end
    end

    def validate_redis_ha_pod_security(context, workload, label)
      pod_spec = workload.pod_spec
      pod_security = pod_spec["securityContext"] || {}
      context.add_error("#{label} must not mount service account tokens") unless pod_spec["automountServiceAccountToken"] == false
      context.add_error("#{label} must run as non-root") unless pod_security["runAsNonRoot"] == true
      unless pod_security["fsGroupChangePolicy"] == "OnRootMismatch"
        context.add_error("#{label} must use OnRootMismatch fsGroup changes")
      end
      unless pod_security.dig("seccompProfile", "type") == "RuntimeDefault"
        context.add_error("#{label} must use RuntimeDefault seccomp")
      end
    end

    def validate_redis_ha_scheduling_spread(context, workload, label)
      pod_spec = workload.pod_spec
      spread = Array(pod_spec["topologySpreadConstraints"]).find do |entry|
        entry["topologyKey"] == "kubernetes.io/hostname" &&
          entry["whenUnsatisfiable"] == "ScheduleAnyway" &&
          redis_ha_component_values(entry.dig("labelSelector", "matchExpressions")).sort == redis_ha_components
      end
      unless spread
        context.add_error("#{label} must prefer hostname topology spread across HA Redis Pods")
      end

      anti_affinity_terms = Array(
        pod_spec.dig("affinity", "podAntiAffinity", "preferredDuringSchedulingIgnoredDuringExecution"),
      )
      anti_affinity = anti_affinity_terms.find do |entry|
        term = entry["podAffinityTerm"] || {}
        term["topologyKey"] == "kubernetes.io/hostname" &&
          redis_ha_component_values(term.dig("labelSelector", "matchExpressions")).sort == redis_ha_components
      end
      unless anti_affinity
        context.add_error("#{label} must prefer hostname anti-affinity across HA Redis Pods")
      end
    end

    def validate_redis_ha_container_security(context, container, label)
      security = container["securityContext"] || {}
      context.add_error("#{label} must forbid privilege escalation") unless security["allowPrivilegeEscalation"] == false
      drops = Array(security.dig("capabilities", "drop"))
      context.add_error("#{label} must drop all Linux capabilities") unless drops.include?("ALL")
    end

    def validate_redis_ha_config_mount(context, stateful_set, container, name)
      mounts = Array(container["volumeMounts"])
      config_mount = mounts.find do |mount|
        mount["name"] == "redis-ha-config" &&
          mount["mountPath"] == "/usr/local/etc/redis" &&
          mount["readOnly"] == true
      end
      context.add_error("non-prod HA Redis #{name} container must mount redis-ha-config read-only") unless config_mount

      volumes = Array(stateful_set.pod_spec["volumes"])
      config_volume = volumes.find { |volume| volume.dig("configMap", "name") == "redis-ha-config" }
      context.add_error("non-prod HA Redis #{name} StatefulSet must mount redis-ha-config") unless config_volume
    end

    def validate_redis_ha_password_ref(context, container, label)
      env = Array(container["env"])
      secret = env.find { |entry| entry["name"] == "REDIS_PASSWORD" }&.dig("valueFrom", "secretKeyRef")
      unless secret
        context.add_error("#{label} must read REDIS_PASSWORD from mpp-app-secrets")
        return
      end

      context.add_error("#{label} must read REDIS_PASSWORD from mpp-app-secrets") unless secret["name"] == "mpp-app-secrets"
      context.add_error("#{label} must read REDIS_PASSWORD from REDIS_PASSWORD") unless secret["key"] == "REDIS_PASSWORD"
      context.add_error("#{label} REDIS_PASSWORD optional flag is wrong") unless secret["optional"] == true
    end

    def validate_redis_ha_readiness(context, container, name)
      command = Array(container.dig("readinessProbe", "exec", "command")).join("\n")
      unless command.include?("redis-cli") && command.match?(/\bping\b/i)
        context.add_error("non-prod HA Redis #{name} readinessProbe must run redis-cli PING")
      end
      unless command.include?("REDIS_PASSWORD")
        context.add_error("non-prod HA Redis #{name} readinessProbe must support optional REDIS_PASSWORD")
      end

      unless command.include?("ROLE") && command.include?('role="$(redis_cli ROLE | head -n 1)"')
        context.add_error("non-prod HA Redis #{name} readinessProbe must inspect the current Redis role")
      end
      unless command.include?('[ "$role" = "master" ]')
        context.add_error("non-prod HA Redis #{name} readinessProbe must accept promoted master role")
      end
      unless command.include?('[ "$role" = "slave" ]') &&
             command.include?('[ "$role" = "replica" ]') &&
             command.include?("INFO replication") &&
             command.include?("master_link_status:up")
        context.add_error("non-prod HA Redis #{name} readinessProbe must verify healthy replica links")
      end
    end

    def redis_ha_component_values(match_expressions)
      Array(match_expressions)
        .select { |entry| entry["key"] == "app.kubernetes.io/component" }
        .flat_map { |entry| Array(entry["values"]) }
        .sort
    end

    def redis_ha_components
      ["redis-ha-primary", "redis-ha-replica", "redis-ha-sentinel"]
    end

    def validate_redis_ha_graceful_shutdown(context, stateful_set, container, name)
      if stateful_set.pod_spec["terminationGracePeriodSeconds"].to_i < 60
        context.add_error("non-prod HA Redis #{name} StatefulSet must allow at least 60 seconds for graceful termination")
      end

      pre_stop_command = Array(container.dig("lifecycle", "preStop", "exec", "command")).join("\n")
      unless pre_stop_command.include?("redis-cli") && pre_stop_command.match?(/\bSHUTDOWN\s+SAVE\b/i)
        context.add_error("non-prod HA Redis #{name} container must run SHUTDOWN SAVE before termination")
      end
      unless pre_stop_command.include?("REDIS_PASSWORD")
        context.add_error("non-prod HA Redis #{name} shutdown hook must support optional REDIS_PASSWORD")
      end
    end

    def validate_redis_ha_data_claim(context, stateful_set, data_claim, name)
      pvc = Array(stateful_set.spec["volumeClaimTemplates"]).find do |claim|
        claim.dig("metadata", "name") == data_claim
      end
      unless pvc
        context.add_error("non-prod HA Redis #{name} StatefulSet must define #{data_claim} persistent storage")
        return
      end

      access_modes = Array(pvc.dig("spec", "accessModes"))
      storage = pvc.dig("spec", "resources", "requests", "storage")
      context.add_error("non-prod HA Redis #{data_claim} PVC must use ReadWriteOnce storage") unless access_modes.include?("ReadWriteOnce")
      context.add_error("non-prod HA Redis #{data_claim} PVC must request storage") if storage.to_s.empty?
    end

    def validate_redis_ha_network_policy(context)
      policy = context.require_document("NetworkPolicy", "redis-ha-internal-access", "mpp-system")
      return unless policy

      selector = policy.spec.dig("podSelector", "matchExpressions") || []
      component_expression = selector.find { |entry| entry["key"] == "app.kubernetes.io/component" }
      components = Array(component_expression&.fetch("values", nil))
      ["redis-ha-primary", "redis-ha-replica", "redis-ha-sentinel"].each do |component|
        unless components.include?(component)
          context.add_error("non-prod HA Redis NetworkPolicy must select #{component} Pods")
        end
      end

      from_components = Array(policy.spec["ingress"]).flat_map { |rule| Array(rule["from"]) }
        .flat_map { |entry| Array(entry.dig("podSelector", "matchExpressions")) }
        .select { |entry| entry["key"] == "app.kubernetes.io/component" }
        .flat_map { |entry| Array(entry["values"]) }
      ["redis-ha-primary", "redis-ha-replica", "redis-ha-sentinel", "redis", "backend", "publish-worker", "browser-worker", "collab-service"].each do |component|
        unless from_components.include?(component)
          context.add_error("non-prod HA Redis NetworkPolicy must allow #{component} ingress")
        end
      end

      ports = Array(policy.spec["ingress"]).flat_map { |rule| Array(rule["ports"]) }.map { |entry| entry["port"] }
      context.add_error("non-prod HA Redis NetworkPolicy must allow Redis port 6379") unless ports.include?(6379)
      context.add_error("non-prod HA Redis NetworkPolicy must allow Sentinel port 26379") unless ports.include?(26379)
    end

    def validate_redis_ha_keeps_existing_traffic(context)
      app_config = context.document("ConfigMap", "mpp-app-config", "mpp-system")
      if app_config
        if app_config.data["REDIS_ADDR"] != "redis:6379"
          context.add_error("non-prod HA Redis validation must keep app REDIS_ADDR on existing redis:6379")
        end

        case app_config.data["REDIS_ENDPOINT_MODE"]
        when nil, "", "direct"
        when "sentinel"
          if app_config.data["REDIS_SENTINEL_ADDRS"].to_s != "redis-ha-sentinel:26379"
            context.add_error("non-prod HA Redis validation must point REDIS_SENTINEL_ADDRS at redis-ha-sentinel:26379")
          end
          if app_config.data["REDIS_SENTINEL_MASTER_NAME"].to_s != "mpp-redis-ha"
            context.add_error("non-prod HA Redis validation must keep REDIS_SENTINEL_MASTER_NAME on mpp-redis-ha")
          end
        else
          context.add_error("non-prod HA Redis validation must use direct or sentinel endpoint mode")
        end
      end

      existing_redis = context.document("Service", "redis", "mpp-system")
      return unless existing_redis

      selector = existing_redis.spec["selector"] || {}
      unless selector["app.kubernetes.io/component"] == "redis"
        context.add_error("non-prod HA Redis validation must leave existing redis Service on the old Redis Pods")
      end
    end

    def validate_redis_ha_production_traffic(context)
      app_config = context.document("ConfigMap", "mpp-app-config", "mpp-system")
      if app_config
        unless app_config.data["APP_ENV"] == "production"
          context.add_error("production HA Redis cutover must run with APP_ENV=production")
        end
        unless app_config.data["REDIS_ENDPOINT_MODE"] == "sentinel"
          context.add_error("production HA Redis cutover must use REDIS_ENDPOINT_MODE=sentinel")
        end
        unless app_config.data["REDIS_SENTINEL_ADDRS"].to_s == "redis-ha-sentinel:26379"
          context.add_error("production HA Redis cutover must point REDIS_SENTINEL_ADDRS at redis-ha-sentinel:26379")
        end
        unless app_config.data["REDIS_SENTINEL_MASTER_NAME"].to_s == "mpp-redis-ha"
          context.add_error("production HA Redis cutover must keep REDIS_SENTINEL_MASTER_NAME on mpp-redis-ha")
        end
        unless app_config.data["REDIS_ADDR"] == "redis:6379"
          context.add_error("production HA Redis cutover must keep REDIS_ADDR=redis:6379 as the direct rollback endpoint")
        end
        unless app_config.data["REDIS_TLS"] == "false"
          context.add_error("production HA Redis cutover must keep REDIS_TLS=false for in-cluster self-hosted Redis")
        end
      end

      existing_redis = context.document("Service", "redis", "mpp-system")
      if existing_redis
        selector = existing_redis.spec["selector"] || {}
        unless selector["app.kubernetes.io/component"] == "redis"
          context.add_error("production HA Redis cutover must leave the old redis Service available for rollback")
        end
      end

      exporter = context.document("Deployment", "redis-exporter", "mpp-system")
      if exporter
        container = exporter.container("redis-exporter")
        env = Array(container&.fetch("env", nil)).each_with_object({}) do |entry, result|
          result[entry["name"]] = entry["value"]
        end
        unless env["REDIS_ADDR"].to_s == "redis://redis-ha-primary.mpp-system.svc.cluster.local:6379"
          context.add_error("production HA Redis cutover must monitor redis-ha-primary through redis-exporter")
        end
      end

      validate_redis_ha_production_network_policy(context)
    end

    def validate_redis_ha_production_network_policy(context)
      policy = context.document("NetworkPolicy", "redis-ha-internal-access", "mpp-system")
      return unless policy

      from_components = Array(policy.spec["ingress"]).flat_map { |rule| Array(rule["from"]) }
        .flat_map { |entry| Array(entry.dig("podSelector", "matchExpressions")) }
        .select { |entry| entry["key"] == "app.kubernetes.io/component" }
        .flat_map { |entry| Array(entry["values"]) }
      unless from_components.include?("redis-exporter")
        context.add_error("production HA Redis NetworkPolicy must allow redis-exporter ingress")
      end
    end

    def validate_redis_exporter_security(context, container)
      security = container["securityContext"] || {}
      context.add_error("redis-exporter container must forbid privilege escalation") unless security["allowPrivilegeEscalation"] == false
      drops = Array(security.dig("capabilities", "drop"))
      context.add_error("redis-exporter container must drop all Linux capabilities") unless drops.include?("ALL")
    end

    def validate_redis_exporter_env(context, container)
      env = Array(container["env"]).each_with_object({}) do |entry, result|
        result[entry["name"]] = entry
      end

      redis_addr = env.dig("REDIS_ADDR", "value").to_s
      unless redis_addr.start_with?("redis://", "rediss://")
        context.add_error("redis-exporter REDIS_ADDR must use redis:// or rediss://")
      end

      secret = env.dig("REDIS_PASSWORD", "valueFrom", "secretKeyRef")
      unless secret
        context.add_error("redis-exporter must read REDIS_PASSWORD from mpp-app-secrets")
        return
      end
      context.add_error("redis-exporter must read REDIS_PASSWORD from mpp-app-secrets") unless secret["name"] == "mpp-app-secrets"
      context.add_error("redis-exporter must read REDIS_PASSWORD from REDIS_PASSWORD") unless secret["key"] == "REDIS_PASSWORD"
      context.add_error("redis-exporter REDIS_PASSWORD optional flag is wrong") unless secret["optional"] == true
    end

    def validate_redis_exporter_probes(context, container)
      ["readinessProbe", "livenessProbe"].each do |probe_name|
        probe = container[probe_name] || {}
        unless probe.dig("httpGet", "path") == "/metrics" && probe.dig("httpGet", "port") == "metrics"
          context.add_error("redis-exporter container #{probe_name} must check /metrics on the metrics port")
        end
      end
    end

    def validate_container(context, container, label)
      context.add_error("#{label} must define readinessProbe") unless container.key?("readinessProbe")
      context.add_error("#{label} must define livenessProbe") unless container.key?("livenessProbe")

      validate_resources(context, container, label)
    end

    def validate_self_hosted_redis_persistence(context, stateful_set)
      config = context.require_document("ConfigMap", "redis-persistence-config", "mpp-system")
      if config
        redis_config = redis_config_lines(config.data["redis.conf"])
        validate_redis_runtime_common_policy(context, redis_config)
        validate_redis_persistence_common_policy(context, redis_config)
        if context.path_suffix?("deploy/kubernetes/data-services/self-hosted")
          validate_base_redis_runtime_policy(context, redis_config)
          validate_base_redis_persistence_policy(context, redis_config)
        else
          validate_overlay_redis_persistence_policy(context, redis_config)
        end
      end

      redis_container = stateful_set.container("redis")
      unless redis_container
        context.add_error("self-hosted redis StatefulSet must define a redis container")
        return
      end

      args_text = Array(redis_container["args"]).join("\n")
      unless args_text.include?("redis-server /usr/local/etc/redis/redis.conf")
        context.add_error("self-hosted redis container must start redis-server with redis.conf")
      end

      mounts = Array(redis_container["volumeMounts"])
      config_mount = mounts.find do |mount|
        mount["name"] == "redis-persistence-config" &&
          mount["mountPath"] == "/usr/local/etc/redis" &&
          mount["readOnly"] == true
      end
      data_mount = mounts.find { |mount| mount["name"] == "redis-data" && mount["mountPath"] == "/data" }
      context.add_error("self-hosted redis container must mount persistence config read-only") unless config_mount
      context.add_error("self-hosted redis container must mount persistent data at /data") unless data_mount

      volumes = Array(stateful_set.pod_spec["volumes"])
      config_volume = volumes.find { |volume| volume.dig("configMap", "name") == "redis-persistence-config" }
      context.add_error("self-hosted redis StatefulSet must mount redis-persistence-config") unless config_volume

      validate_self_hosted_redis_runtime_hardening(context, stateful_set, redis_container)

      redis_pvc = Array(stateful_set.spec["volumeClaimTemplates"]).find do |claim|
        claim.dig("metadata", "name") == "redis-data"
      end
      unless redis_pvc
        context.add_error("self-hosted redis StatefulSet must define redis-data persistent storage")
        return
      end

      access_modes = Array(redis_pvc.dig("spec", "accessModes"))
      storage = redis_pvc.dig("spec", "resources", "requests", "storage")
      context.add_error("self-hosted redis-data PVC must use ReadWriteOnce storage") unless access_modes.include?("ReadWriteOnce")
      context.add_error("self-hosted redis-data PVC must request storage") if storage.to_s.empty?
    end

    def validate_self_hosted_redis_runtime_hardening(context, stateful_set, redis_container)
      grace_period = stateful_set.pod_spec["terminationGracePeriodSeconds"].to_i
      if grace_period < 60
        context.add_error("self-hosted redis StatefulSet must allow at least 60 seconds for graceful termination")
      end

      ["readinessProbe", "livenessProbe"].each do |probe_name|
        validate_redis_ping_probe(context, redis_container, probe_name)
      end

      pre_stop_command = Array(redis_container.dig("lifecycle", "preStop", "exec", "command")).join("\n")
      unless pre_stop_command.include?("redis-cli") && pre_stop_command.match?(/\bSHUTDOWN\s+SAVE\b/i)
        context.add_error("self-hosted redis container must run SHUTDOWN SAVE before termination")
      end
      unless pre_stop_command.include?("REDIS_PASSWORD")
        context.add_error("self-hosted redis shutdown hook must support optional REDIS_PASSWORD")
      end
    end

    def validate_redis_ping_probe(context, container, probe_name)
      command = Array(container.dig(probe_name, "exec", "command")).join("\n")
      unless command.include?("redis-cli") && command.match?(/\bping\b/i)
        context.add_error("self-hosted redis container #{probe_name} must run redis-cli PING")
      end
      unless command.include?("REDIS_PASSWORD")
        context.add_error("self-hosted redis container #{probe_name} must support optional REDIS_PASSWORD")
      end
    end

    def validate_redis_persistence_common_policy(context, redis_config)
      unless redis_config.include?("dir /data")
        context.add_error("self-hosted redis persistence config must write data under /data")
      end
    end

    def validate_redis_runtime_common_policy(context, redis_config)
      maxmemory = redis_config_value(redis_config, "maxmemory")
      if maxmemory.nil?
        context.add_error("self-hosted redis runtime config must explicitly set maxmemory")
      elsif !positive_redis_memory?(maxmemory)
        context.add_error("self-hosted redis runtime config maxmemory must be greater than zero")
      end

      maxmemory_policy = redis_config_value(redis_config, "maxmemory-policy")
      if maxmemory_policy.nil?
        context.add_error("self-hosted redis runtime config must explicitly set maxmemory-policy")
      elsif !VALID_REDIS_MAXMEMORY_POLICIES.include?(maxmemory_policy)
        context.add_error("self-hosted redis runtime config maxmemory-policy must be a valid Redis eviction policy")
      end

      timeout = redis_config_integer(redis_config, "timeout")
      if timeout.nil?
        context.add_error("self-hosted redis runtime config must set non-negative timeout")
      end

      tcp_keepalive = redis_config_integer(redis_config, "tcp-keepalive")
      if tcp_keepalive.nil? || tcp_keepalive <= 0
        context.add_error("self-hosted redis runtime config must enable tcp-keepalive")
      end

      slowlog_threshold = redis_config_integer(redis_config, "slowlog-log-slower-than")
      if slowlog_threshold.nil?
        context.add_error("self-hosted redis runtime config must set non-negative slowlog-log-slower-than")
      end

      slowlog_max_len = redis_config_integer(redis_config, "slowlog-max-len")
      if slowlog_max_len.nil? || slowlog_max_len <= 0
        context.add_error("self-hosted redis runtime config must retain slowlog entries")
      end
    end

    def validate_base_redis_runtime_policy(context, redis_config)
      {
        "maxmemory" => ["384mb", "self-hosted redis runtime config must keep maxmemory at 384mb"],
        "maxmemory-policy" => ["noeviction", "self-hosted redis runtime config must use noeviction"],
        "timeout" => ["0", "self-hosted redis runtime config must keep idle timeout disabled"],
        "tcp-keepalive" => ["300", "self-hosted redis runtime config must keep tcp-keepalive at 300 seconds"],
        "slowlog-log-slower-than" => ["10000", "self-hosted redis runtime config must log commands slower than 10ms"],
        "slowlog-max-len" => ["256", "self-hosted redis runtime config must retain 256 slowlog entries"],
      }.each do |key, (expected_value, message)|
        context.add_error(message) unless redis_config_value(redis_config, key) == expected_value
      end
    end

    def validate_base_redis_persistence_policy(context, redis_config)
      {
        "appendonly yes" => "self-hosted redis persistence config must enable AOF",
        "appendfsync everysec" => "self-hosted redis persistence config must fsync AOF every second",
        "save 900 1" => "self-hosted redis persistence config must keep the long-window RDB snapshot policy",
        "save 300 10" => "self-hosted redis persistence config must keep the medium-window RDB snapshot policy",
        "save 60 10000" => "self-hosted redis persistence config must keep the burst RDB snapshot policy",
      }.each do |line, message|
        context.add_error(message) unless redis_config.include?(line)
      end
    end

    def validate_overlay_redis_persistence_policy(context, redis_config)
      appendonly = redis_config_value(redis_config, "appendonly")
      if appendonly.nil?
        context.add_error("self-hosted redis persistence config must explicitly choose appendonly yes or no")
        return
      end

      unless ["yes", "no"].include?(appendonly)
        context.add_error("self-hosted redis persistence config appendonly must be yes or no")
      end

      if appendonly == "yes"
        appendfsync = redis_config_value(redis_config, "appendfsync")
        unless ["always", "everysec", "no"].include?(appendfsync)
          context.add_error("self-hosted redis persistence config must choose a valid appendfsync policy when AOF is enabled")
        end
      end

      unless appendonly == "yes" || active_redis_save_policy?(redis_config)
        context.add_error("self-hosted redis persistence config must enable AOF or at least one RDB save policy")
      end
    end

    def redis_config_lines(config)
      config.to_s.lines.map(&:strip).reject { |line| line.empty? || line.start_with?("#") }
    end

    def redis_config_value(redis_config, key)
      line = redis_config.reverse.find { |entry| entry.start_with?("#{key} ") }
      line&.split(/\s+/, 2)&.fetch(1, nil)&.delete_prefix('"')&.delete_suffix('"')
    end

    def redis_config_integer(redis_config, key)
      value = redis_config_value(redis_config, key)
      return nil unless value&.match?(/\A\d+\z/)

      value.to_i
    end

    def positive_redis_memory?(value)
      value.to_s.match?(/\A[1-9]\d*(?:[kmg]b?)?\z/i)
    end

    def active_redis_save_policy?(redis_config)
      redis_config.any? do |line|
        next false unless line.start_with?("save ")

        value = line.split(/\s+/, 2).fetch(1, "").delete_prefix('"').delete_suffix('"')
        !value.empty?
      end
    end

    def validate_resources(context, container, label)
      requests = container.dig("resources", "requests") || {}
      limits = container.dig("resources", "limits") || {}
      ["cpu", "memory"].each do |resource|
        context.add_error("#{label} must define #{resource} requests") unless requests.key?(resource)
        context.add_error("#{label} must define #{resource} limits") unless limits.key?(resource)
      end
    end

    def validate_self_hosted_read_replica(context)
      stateful_set = context.require_document("StatefulSet", "postgres-read-replica", "mpp-system")
      return unless stateful_set

      unless stateful_set.spec["serviceName"] == "postgres-reader"
        context.add_error("self-hosted postgres-read-replica StatefulSet must use postgres-reader service")
      end

      selector_component = stateful_set.spec.dig("selector", "matchLabels", "app.kubernetes.io/component")
      pod_component = stateful_set.spec.dig("template", "metadata", "labels", "app.kubernetes.io/component")
      unless selector_component == "postgres-reader" && pod_component == "postgres-reader"
        context.add_error("self-hosted postgres-read-replica StatefulSet must select postgres-reader Pods")
      end

      pod_spec = stateful_set.pod_spec
      pod_security = pod_spec["securityContext"] || {}
      context.add_error("self-hosted postgres-read-replica StatefulSet must not mount service account tokens") unless pod_spec["automountServiceAccountToken"] == false
      context.add_error("self-hosted postgres-read-replica StatefulSet must run as non-root") unless pod_security["runAsNonRoot"] == true
      unless pod_security["fsGroupChangePolicy"] == "OnRootMismatch"
        context.add_error("self-hosted postgres-read-replica StatefulSet must use OnRootMismatch fsGroup changes")
      end
      unless Array(stateful_set.spec["volumeClaimTemplates"]).any?
        context.add_error("self-hosted postgres-read-replica StatefulSet must define persistent storage")
      end

      stateful_set.containers.each do |container|
        validate_container(context, container, "self-hosted postgres-read-replica container #{container['name']}")
      end

      init_container = Array(pod_spec["initContainers"]).find { |container| container["name"] == "clone-primary" }
      unless init_container
        context.add_error("self-hosted postgres-read-replica StatefulSet must clone primary before startup")
        return
      end

      validate_resources(context, init_container, "self-hosted postgres-read-replica init container clone-primary")
      command_text = Array(init_container["command"]).join("\n")
      unless command_text.include?("pg_basebackup")
        context.add_error("self-hosted postgres-read-replica init container must use pg_basebackup")
      end
    end

    def validate_self_hosted_backups(context)
      scripts = context.require_document("ConfigMap", "mpp-data-backup-scripts", "mpp-system")
      if scripts
        ["postgres-backup.sh", "redis-backup.sh"].each do |script_name|
          unless scripts.data.key?(script_name)
            context.add_error("self-hosted backup scripts ConfigMap must include #{script_name}")
          end
        end
        validate_redis_backup_script(context, scripts)
      end

      backup_pvc = context.require_document("PersistentVolumeClaim", "mpp-data-backups", "mpp-system")
      if backup_pvc
        access_modes = Array(backup_pvc.spec["accessModes"])
        storage = backup_pvc.spec.dig("resources", "requests", "storage")
        context.add_error("self-hosted backup PVC must use ReadWriteOnce storage") unless access_modes.include?("ReadWriteOnce")
        context.add_error("self-hosted backup PVC must request storage") if storage.to_s.empty?
      end

      validate_backup_cronjob(
        context,
        name: "postgres-backup",
        image: "docker.io/library/postgres:18.4",
        secret_env: "PGPASSWORD",
        secret_key: "DB_PASSWORD",
        optional_secret: false,
        max_deadline_seconds: 1800,
      )
      validate_backup_cronjob(
        context,
        name: "redis-backup",
        image: "docker.io/library/redis:7-alpine",
        secret_env: "REDIS_PASSWORD",
        secret_key: "REDIS_PASSWORD",
        optional_secret: true,
        max_deadline_seconds: 900,
      )
    end

    def validate_backup_cronjob(context, name:, image:, secret_env:, secret_key:, optional_secret:, max_deadline_seconds:)
      cronjob = context.require_document("CronJob", name, "mpp-system")
      return unless cronjob

      context.add_error("self-hosted #{name} CronJob must define a schedule") if cronjob.spec["schedule"].to_s.empty?
      context.add_error("self-hosted #{name} CronJob must forbid concurrent runs") unless cronjob.spec["concurrencyPolicy"] == "Forbid"

      job_spec = cronjob.spec.dig("jobTemplate", "spec") || {}
      unless job_spec["activeDeadlineSeconds"].to_i.between?(1, max_deadline_seconds)
        context.add_error("self-hosted #{name} CronJob must set activeDeadlineSeconds <= #{max_deadline_seconds}")
      end
      context.add_error("self-hosted #{name} CronJob must keep failed job history") unless job_spec["failedJobsHistoryLimit"] || cronjob.spec["failedJobsHistoryLimit"]
      context.add_error("self-hosted #{name} CronJob must keep successful job history") unless job_spec["successfulJobsHistoryLimit"] || cronjob.spec["successfulJobsHistoryLimit"]

      pod_spec = job_spec.dig("template", "spec") || {}
      labels = job_spec.dig("template", "metadata", "labels") || {}
      unless labels["app.kubernetes.io/component"] == name
        context.add_error("self-hosted #{name} CronJob Pods must carry the #{name} component label")
      end

      validate_backup_pod_security(context, pod_spec, name)

      container = Array(pod_spec["containers"]).find { |entry| entry["name"] == name }
      unless container
        context.add_error("self-hosted #{name} CronJob must define a #{name} container")
        return
      end

      context.add_error("self-hosted #{name} container must use #{image}") unless container["image"] == image
      validate_resources(context, container, "self-hosted #{name} container")
      validate_backup_container_security(context, container, name)
      validate_backup_volume_mounts(context, container, name)
      validate_backup_secret_ref(context, container, name, secret_env, secret_key, optional_secret)
    end

    def validate_redis_backup_script(context, scripts)
      script = scripts.data["redis-backup.sh"].to_s
      return if script.empty?

      {
        "/backups/redis" => "self-hosted redis backup script must default BACKUP_ROOT to /backups/redis",
        "--rdb \"$tmp_file\"" => "self-hosted redis backup script must stream RDB snapshots with redis-cli --rdb",
        "REDISCLI_AUTH" => "self-hosted redis backup script must support optional REDIS_PASSWORD auth",
        "mv \"$tmp_file\" \"$target_file\"" => "self-hosted redis backup script must atomically publish complete snapshots",
        "find \"$backup_root\" -type f -name \"redis-*.rdb\"" => "self-hosted redis backup script must prune retained Redis snapshots",
      }.each do |needle, message|
        context.add_error(message) unless script.include?(needle)
      end
    end

    def validate_backup_pod_security(context, pod_spec, name)
      pod_security = pod_spec["securityContext"] || {}
      context.add_error("self-hosted #{name} CronJob must not mount service account tokens") unless pod_spec["automountServiceAccountToken"] == false
      context.add_error("self-hosted #{name} CronJob must not restart failed Pods") unless pod_spec["restartPolicy"] == "Never"
      context.add_error("self-hosted #{name} CronJob must run as non-root") unless pod_security["runAsNonRoot"] == true
      unless pod_security["fsGroupChangePolicy"] == "OnRootMismatch"
        context.add_error("self-hosted #{name} CronJob must use OnRootMismatch fsGroup changes")
      end
      unless pod_security.dig("seccompProfile", "type") == "RuntimeDefault"
        context.add_error("self-hosted #{name} CronJob must use RuntimeDefault seccomp")
      end

      volumes = Array(pod_spec["volumes"])
      backup_volume = volumes.find { |volume| volume.dig("persistentVolumeClaim", "claimName") == "mpp-data-backups" }
      scripts_volume = volumes.find { |volume| volume.dig("configMap", "name") == "mpp-data-backup-scripts" }
      context.add_error("self-hosted #{name} CronJob must mount the backup PVC") unless backup_volume
      context.add_error("self-hosted #{name} CronJob must mount backup scripts") unless scripts_volume
    end

    def validate_backup_container_security(context, container, name)
      security = container["securityContext"] || {}
      context.add_error("self-hosted #{name} container must forbid privilege escalation") unless security["allowPrivilegeEscalation"] == false
      drops = Array(security.dig("capabilities", "drop"))
      context.add_error("self-hosted #{name} container must drop all Linux capabilities") unless drops.include?("ALL")
    end

    def validate_backup_volume_mounts(context, container, name)
      mounts = Array(container["volumeMounts"])
      backups_mount = mounts.find { |mount| mount["name"] == "data-backups" && mount["mountPath"] == "/backups" }
      scripts_mount = mounts.find { |mount| mount["name"] == "backup-scripts" && mount["mountPath"] == "/scripts" && mount["readOnly"] == true }
      context.add_error("self-hosted #{name} container must mount backup storage at /backups") unless backups_mount
      context.add_error("self-hosted #{name} container must mount scripts read-only at /scripts") unless scripts_mount
    end

    def validate_backup_secret_ref(context, container, name, secret_env, secret_key, optional_secret)
      env = Array(container["env"])
      secret = env.find { |entry| entry["name"] == secret_env }&.dig("valueFrom", "secretKeyRef")
      unless secret
        context.add_error("self-hosted #{name} container must read #{secret_env} from mpp-app-secrets")
        return
      end

      context.add_error("self-hosted #{name} container must read #{secret_env} from mpp-app-secrets") unless secret["name"] == "mpp-app-secrets"
      context.add_error("self-hosted #{name} container must read #{secret_env} from #{secret_key}") unless secret["key"] == secret_key
      context.add_error("self-hosted #{name} container #{secret_env} optional flag is wrong") unless secret.fetch("optional", false) == optional_secret
    end

    def validate_self_hosted_network_policy(context, name, port, allowed_components)
      policy = context.require_document("NetworkPolicy", "#{name}-app-access", "mpp-system")
      return unless policy

      selector = policy.spec.dig("podSelector", "matchLabels") || {}
      unless selector["app.kubernetes.io/component"] == name
        context.add_error("self-hosted #{name} NetworkPolicy must select #{name} Pods")
      end

      types = Array(policy.spec["policyTypes"])
      context.add_error("self-hosted #{name} NetworkPolicy must be an ingress policy") unless types.include?("Ingress")

      from_entries = Array(policy.spec["ingress"]).flat_map { |rule| Array(rule["from"]) }
      components = from_entries.map { |entry| entry.dig("podSelector", "matchLabels", "app.kubernetes.io/component") }.compact
      allowed_components.each do |component|
        unless components.include?(component)
          context.add_error("self-hosted #{name} NetworkPolicy must allow #{component} ingress")
        end
      end

      ports = Array(policy.spec["ingress"]).flat_map { |rule| Array(rule["ports"]) }.map { |entry| entry["port"] }
      context.add_error("self-hosted #{name} NetworkPolicy must target port #{port}") unless ports.include?(port)
    end
  end
end
