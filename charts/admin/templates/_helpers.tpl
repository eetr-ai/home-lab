{{- define "home-lab-admin.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: home-lab-admin
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{- /*
The API image, addressed by an explicit tag. There is deliberately no default of
"latest": a pod pulling a moving name cannot say which build it is running, and
the chart's appVersion is what release-please keeps in step with the images.
*/ -}}
{{- define "home-lab-admin.apiImage" -}}
{{- $image := .Values.admin.api.image -}}
{{- printf "%s:%s" $image.repository (required "admin.api.image.tag is required" $image.tag) -}}
{{- end }}
