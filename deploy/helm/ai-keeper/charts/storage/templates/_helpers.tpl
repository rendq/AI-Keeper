{{/*
Helper templates for the `storage` sub-chart.
*/}}

{{- define "storage.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "storage.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $n := default .Chart.Name .Values.nameOverride -}}
{{- if contains $n .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $n | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "storage.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "storage.labels" -}}
helm.sh/chart: {{ include "storage.chart" . }}
app.kubernetes.io/name: {{ include "storage.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: aip
{{- end -}}

{{- define "storage.selectorLabels" -}}
app.kubernetes.io/name: {{ include "storage.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* PostgreSQL specific labels */}}
{{- define "storage.postgresql.labels" -}}
{{ include "storage.labels" . }}
app.kubernetes.io/component: postgresql
app: postgresql
{{- end -}}

{{- define "storage.postgresql.selectorLabels" -}}
{{ include "storage.selectorLabels" . }}
app.kubernetes.io/component: postgresql
app: postgresql
{{- end -}}

{{/* Redis specific labels */}}
{{- define "storage.redis.labels" -}}
{{ include "storage.labels" . }}
app.kubernetes.io/component: redis
app: redis
{{- end -}}

{{- define "storage.redis.selectorLabels" -}}
{{ include "storage.selectorLabels" . }}
app.kubernetes.io/component: redis
app: redis
{{- end -}}

{{/* NATS specific labels */}}
{{- define "storage.nats.labels" -}}
{{ include "storage.labels" . }}
app.kubernetes.io/component: nats
app: nats
{{- end -}}

{{- define "storage.nats.selectorLabels" -}}
{{ include "storage.selectorLabels" . }}
app.kubernetes.io/component: nats
app: nats
{{- end -}}
