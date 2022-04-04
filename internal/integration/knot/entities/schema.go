package entities

// Schema represents the thing's schema
type Schema struct {
	ValueType int    `mapstructure: "valueType" json:"valueType" validate:"required"`
	Unit      int    `mapstructure: "unit" json:"unit"`
	TypeID    int    `mapstructure: "typeId" json:"typeId" validate:"required"`
	Name      string `mapstructure: "name" json:"name" validate:"required,max=30"`
}
