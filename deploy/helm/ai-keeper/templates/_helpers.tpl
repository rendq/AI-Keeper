{{/*
Common helper templates for the AIP umbrella chart.

Sub-charts include these by relying on the standard Helm "$.<name>" lookup;
each sub-chart also ships its own thin _helpers.tpl in `charts/<name>/templates`
so they remain installable on their own.
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "aip.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Compute a fully-qualified app name. We avoid double-prefixing if release name
already contains the chart name. Truncated to 63 chars (DNS-1123 label).
*/}}
{{- define "aip.fullname" -}}
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

{{/*
Chart label "name-version".
*/}}
{{- define "aip.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Standard label set: emitted by every AIP-managed object so they can be
selected per-tenant / per-component / per-version.
*/}}
{{- define "aip.labels" -}}
helm.sh/chart: {{ include "aip.chart" . }}
app.kubernetes.io/name: {{ include "aip.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: aip
{{- end -}}

{{/*
Selector labels — the subset of `aip.labels` allowed on a Service / Deployment
selector (cannot include version/chart, those are not selector-stable).
*/}}
{{- define "aip.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aip.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Resolve the global image tag, falling back to .Chart.AppVersion if a value
is empty. Used as `{{ include "aip.imageTag" (dict "tag" .Values.image.tag "ctx" $) }}`.
*/}}
{{- define "aip.imageTag" -}}
{{- $tag := .tag -}}
{{- if $tag -}}
{{- $tag -}}
{{- else if .ctx.Values.global.imageTag -}}
{{- .ctx.Values.global.imageTag -}}
{{- else -}}
{{- .ctx.Chart.AppVersion -}}
{{- end -}}
{{- end -}}
