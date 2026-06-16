# frozen_string_literal: true

module KubernetesValidation
  module DataServices
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
        {
          "dir /data" => "self-hosted redis persistence config must write data under /data",
          "appendonly yes" => "self-hosted redis persistence config must enable AOF",
          "appendfsync everysec" => "self-hosted redis persistence config must fsync AOF every second",
          "save 900 1" => "self-hosted redis persistence config must keep the long-window RDB snapshot policy",
          "save 300 10" => "self-hosted redis persistence config must keep the medium-window RDB snapshot policy",
          "save 60 10000" => "self-hosted redis persistence config must keep the burst RDB snapshot policy",
        }.each do |line, message|
          context.add_error(message) unless redis_config.include?(line)
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

    def redis_config_lines(config)
      config.to_s.lines.map(&:strip).reject { |line| line.empty? || line.start_with?("#") }
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
