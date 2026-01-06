{{/*
Expand the name of the chart.
*/}}
{{- define "jobplane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "jobplane.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "jobplane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "jobplane.labels" -}}
helm.sh/chart: {{ include "jobplane.chart" . }}
{{ include "jobplane.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "jobplane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "jobplane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "jobplane.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "jobplane.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Controller selector labels
*/}}
{{- define "jobplane.controller.selectorLabels" -}}
{{ include "jobplane.selectorLabels" . }}
app.kubernetes.io/component: controller
{{- end }}

{{/*
Worker selector labels
*/}}
{{- define "jobplane.worker.selectorLabels" -}}
{{ include "jobplane.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}

{{/*
Create the database URL from components
*/}}
{{- define "jobplane.databaseURL" -}}
postgres://{{ .Values.database.user }}:{{ .Values.database.password }}@{{ .Values.database.host }}:{{ .Values.database.port }}/{{ .Values.database.name }}?sslmode={{ .Values.database.sslMode }}
{{- end }}
