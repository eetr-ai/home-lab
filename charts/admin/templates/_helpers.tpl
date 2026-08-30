{{- define "home-lab-admin.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: home-lab-admin
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{- /*
The tag all three images carry, addressed explicitly. There is deliberately no
default of "latest": a pod pulling a moving name cannot say which build it is
running, and the chart's appVersion is what release-please keeps in step with the
images.

One tag rather than three because one pipeline builds all three from one commit.
`required` rather than a default, so a values file that forgot it fails at render
with the key's name instead of installing something nobody chose.
*/ -}}
{{- define "home-lab-admin.imageTag" -}}
{{- required "admin.image.tag is required" .Values.admin.image.tag -}}
{{- end }}

{{- /*
The API image.
*/ -}}
{{- define "home-lab-admin.apiImage" -}}
{{- printf "%s:%s" .Values.admin.api.image.repository (include "home-lab-admin.imageTag" .) -}}
{{- end }}

{{- /*
The panel image, addressed the same way and for the same reason as the API's.
*/ -}}
{{- define "home-lab-admin.webImage" -}}
{{- printf "%s:%s" .Values.admin.web.image.repository (include "home-lab-admin.imageTag" .) -}}
{{- end }}

{{- /*
The agent image, addressed the same way and for the same reason as the other two.
It carries the agent's definition as well as the runtime, so the tag names the
prompt and the tool set and not only the binary.
*/ -}}
{{- define "home-lab-admin.agentImage" -}}
{{- printf "%s:%s" .Values.admin.agent.image.repository (include "home-lab-admin.imageTag" .) -}}
{{- end }}
