{{/*
Expand the name of the chart.
*/}}
{{- define "obarena.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "obarena.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "obarena.labels" -}}
helm.sh/chart: {{ include "obarena.chart" . }}
{{ include "obarena.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "obarena.selectorLabels" -}}
app.kubernetes.io/name: {{ include "obarena.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Image rendering
Resolves to <registry>/<service>:<tag>
If registry is empty, assumes local image resolving to just <service>:<tag>
Usage: {{ include "obarena.image" (list . "submission-api") }}
*/}}
{{- define "obarena.image" -}}
{{- $top := index . 0 -}}
{{- $service := index . 1 -}}
{{- if $top.Values.image.registry -}}
{{ printf "%s/%s:%s" $top.Values.image.registry $service $top.Values.image.tag }}
{{- else -}}
{{ printf "%s:%s" $service $top.Values.image.tag }}
{{- end -}}
{{- end -}}

{{/*
Platform pool scheduling (nodeSelector + tolerations). Omit entirely when null (values-dev).
*/}}
{{- define "obarena.platformScheduling" -}}
{{- with .Values.nodeSelector.platform }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.tolerations.platform }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end -}}
