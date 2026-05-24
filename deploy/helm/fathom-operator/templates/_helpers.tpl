{{- define "fathom-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "fathom-operator.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "fathom-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "fathom-operator.labels" -}}
helm.sh/chart: {{ include "fathom-operator.chart" . }}
{{ include "fathom-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: fathom
{{- end -}}

{{- define "fathom-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "fathom-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "fathom-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "fathom-operator.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- /* Refuse to silently fall back to "default": binding the manager
       ClusterRole to the namespace default SA would over-grant every
       workload using it. Require an explicit, pre-existing SA name. */ -}}
{{- required "serviceAccount.name is required when serviceAccount.create=false (otherwise the manager RBAC would bind to the namespace's \"default\" ServiceAccount and over-grant it)." .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Operator image reference. Tag defaults to v<AppVersion> — fathom image tags
carry a leading v, but the chart's appVersion is plain semver per Helm.
*/}}
{{- define "fathom-operator.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default (printf "v%s" .Chart.AppVersion)) -}}
{{- end -}}

{{/*
Probe image reference passed to the operator via --probe-image. Same leading-v
tag convention as the operator image.
*/}}
{{- define "fathom-operator.probeImage" -}}
{{- printf "%s:%s" .Values.probeImage.repository (.Values.probeImage.tag | default (printf "v%s" .Chart.AppVersion)) -}}
{{- end -}}

{{/*
Extract the numeric port from a controller-runtime bind address. options.go
validates these as host:port (net.SplitHostPort), so values look like ":8443",
"0.0.0.0:8443", or "[::1]:8443" (or "0" to disable) — note a bare "8443" is
rejected by the operator. Split on ":" and take the last segment rather than
assuming the ":<port>" shorthand. Caller pipes the result through `int`.
*/}}
{{- define "fathom-operator.port" -}}
{{- splitList ":" (toString .) | last -}}
{{- end -}}
