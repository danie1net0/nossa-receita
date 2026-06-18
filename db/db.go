package db

const (
	companyTableName = "cnpj"
	graphTableName   = "graph"
	metaTableName    = "meta"
)

// GraphEdge represents a connection between two nodes in the graph.
type GraphEdge struct {
	CompanyID   string `json:"cnpj"`
	CompanyName string `json:"razao_social"`
	PartnerID   string `json:"id"`
	PartnerName string `json:"nome"`
	PartnerCPF  string `json:"cpf,omitempty"`
	PartnerType int    `json:"-"`
}
