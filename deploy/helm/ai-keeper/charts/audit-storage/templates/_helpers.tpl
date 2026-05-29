{{/*
Helper templates for the `audit-storage` sub-chart. Mirrors the umbrella
chart's helpers so this sub-chart is independently installable.
*/}}

{{- define "audit-storage.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "audit-storage.fullname" -}}
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

{{- define "audit-storage.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "audit-storage.labels" -}}
helm.sh/chart: {{ include "audit-storage.chart" . }}
app.kubernetes.io/name: {{ include "audit-storage.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: aip
{{- end -}}

{{- define "audit-storage.selectorLabels" -}}
app.kubernetes.io/name: {{ include "audit-storage.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* ClickHouse specific labels */}}
{{- define "audit-storage.clickhouse.labels" -}}
{{ include "audit-storage.labels" . }}
app.kubernetes.io/component: clickhouse
app: clickhouse
{{- end -}}

{{- define "audit-storage.clickhouse.selectorLabels" -}}
{{ include "audit-storage.selectorLabels" . }}
app.kubernetes.io/component: clickhouse
app: clickhouse
{{- end -}}

{{/* MinIO specific labels */}}
{{- define "audit-storage.minio.labels" -}}
{{ include "audit-storage.labels" . }}
app.kubernetes.io/component: minio
app: minio
{{- end -}}

{{- define "audit-storage.minio.selectorLabels" -}}
{{ include "audit-storage.selectorLabels" . }}
app.kubernetes.io/component: minio
app: minio
{{- end -}}
