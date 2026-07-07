{{ if not .Logged }}
ALTER TABLE {{ .CompanyTableFullName }} SET UNLOGGED;
ALTER TABLE empresa SET UNLOGGED;
ALTER TABLE estabelecimento SET UNLOGGED;
ALTER TABLE simples SET UNLOGGED;
ALTER TABLE socio SET UNLOGGED;
ALTER TABLE estabelecimento_cnae SET UNLOGGED;
ALTER TABLE regime_tributario SET UNLOGGED;
{{ end }}
