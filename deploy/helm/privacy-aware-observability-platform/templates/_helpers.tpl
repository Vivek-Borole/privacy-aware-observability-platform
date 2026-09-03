{{- define "paop.labels" -}}
app.kubernetes.io/part-of: privacy-aware-observability-platform
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
{{- define "paop.selector" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
{{- end }}
