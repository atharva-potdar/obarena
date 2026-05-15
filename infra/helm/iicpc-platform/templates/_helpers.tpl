{{/*
Expand the name of the chart.
*/}}
{{- define "iicpc.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "iicpc.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "iicpc.labels" -}}
helm.sh/chart: {{ include "iicpc.chart" . }}
{{ include "iicpc.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "iicpc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "iicpc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Image rendering
Resolves to <registry>/<service>:<tag>
If registry is empty, assumes local image resolving to just <service>:<tag>
Usage: {{ include "iicpc.image" (list . "submission-api") }}
*/}}
{{- define "iicpc.image" -}}
{{- $top := index . 0 -}}
{{- $service := index . 1 -}}
{{- if $top.Values.image.registry -}}
{{ printf "%s/%s:%s" $top.Values.image.registry $service $top.Values.image.tag }}
{{- else -}}
{{ printf "%s:%s" $service $top.Values.image.tag }}
{{- end -}}
{{- end -}}
