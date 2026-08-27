{{- define "dbx.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "dbx.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "dbx.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "dbx.labels" -}}
app: dbx-orchestrator
app.kubernetes.io/name: {{ include "dbx.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
