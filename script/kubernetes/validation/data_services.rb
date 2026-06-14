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
      end

      validate_self_hosted_read_replica(context)
      validate_self_hosted_backups(context)
      validate_self_hosted_network_policy(context, "postgres", 5432, ["backend", "publish-worker", "collab-service", "pgbouncer", "postgres-backup"])
      validate_self_hosted_network_policy(context, "postgres-reader", 5432, ["pgbouncer-reader"])
      validate_self_hosted_network_policy(context, "pgbouncer", 5432, ["backend", "publish-worker", "collab-service"])
      validate_self_hosted_network_policy(context, "pgbouncer-reader", 5432, ["backend", "publish-worker", "collab-service"])
      validate_self_hosted_network_policy(context, "redis", 6379, ["backend", "publish-worker", "browser-worker", "collab-service", "redis-backup"])
    end

    def validate_container(context, container, label)
      context.add_error("#{label} must define readinessProbe") unless container.key?("readinessProbe")
      context.add_error("#{label} must define livenessProbe") unless container.key?("livenessProbe")

      validate_resources(context, container, label)
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
