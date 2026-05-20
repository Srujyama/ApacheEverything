{{/*
Common labels and naming helpers.
*/}}
{{- define "sunny.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sunny.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "sunny.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "sunny.labels" -}}
app.kubernetes.io/name: {{ include "sunny.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ default .Chart.AppVersion .Values.image.tag | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{/*
Image reference. Pins to a sha256 digest when values.image.digest is set,
otherwise uses repo:tag with tag defaulting to chart appVersion.
*/}}
{{- define "sunny.image" -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{/*
serviceAccountName resolves the SA name. Honors an explicit override; otherwise
uses the chart's fullname.
*/}}
{{- define "sunny.serviceAccountName" -}}
{{- if .Values.serviceAccount.name -}}
{{- .Values.serviceAccount.name -}}
{{- else -}}
{{- include "sunny.fullname" . -}}
{{- end -}}
{{- end -}}

{{/*
hasSecrets returns non-empty when ANY secret value is set. Keeps both the
Secret manifest and the envFrom block in deployment.yaml conditional on
the same predicate.
*/}}
{{- define "sunny.hasSecrets" -}}
{{- if or
    .Values.auth.passwordHash
    .Values.auth.sessionKey
    .Values.auth.apiTokens
    .Values.oidc.issuer
    .Values.oidc.clientId
    .Values.oidc.clientSecret
    .Values.oidc.redirectUrl
    .Values.oidc.scopes
    .Values.alerts.webhookUrl
    .Values.alerts.slackUrl
    .Values.secrets.nasaFirmsKey
    .Values.secrets.openaqApiKey -}}
true
{{- end -}}
{{- end -}}
