package generator

import (
	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"github.com/dave/jennifer/jen"
)

// GenerateInfrastructure generates the converter infrastructure code
func GenerateInfrastructure(f *jen.File, cfg *config.Config, importMap map[string]string) {
	// Converter type
	f.Comment("Converter type for type-safe conversions")
	f.Type().Id("Converter").Types(
		jen.Id("From").Any(),
		jen.Id("To").Any(),
	).Func().Params(jen.Id("From")).Params(jen.Id("To"), jen.Error())

	f.Line()

	// Global registry with mutex for thread-safety
	f.Comment("Global converter registry (thread-safe)")
	f.Var().Id("converters").Op("=").Make(jen.Map(jen.String()).Any())
	f.Var().Id("convertersMu").Qual("sync", "RWMutex")

	f.Line()

	// RegisterConverter
	f.Comment("RegisterConverter registers a type-safe converter")
	f.Func().Id("RegisterConverter").Types(
		jen.Id("From").Any(),
		jen.Id("To").Any(),
	).Params(
		jen.Id("name").String(),
		jen.Id("fn").Id("Converter").Types(jen.Id("From"), jen.Id("To")),
	).Block(
		jen.Id("convertersMu").Dot("Lock").Call(),
		jen.Defer().Id("convertersMu").Dot("Unlock").Call(),
		jen.Id("converters").Index(jen.Id("name")).Op("=").Id("fn"),
	)

	f.Line()

	// Convert
	f.Comment("Convert performs a type-safe conversion using a registered converter")
	f.Func().Id("Convert").Types(
		jen.Id("From").Any(),
		jen.Id("To").Any(),
	).Params(
		jen.Id("name").String(),
		jen.Id("value").Id("From"),
	).Params(jen.Id("To"), jen.Error()).Block(
		jen.Var().Id("zero").Id("To"),
		jen.Id("convertersMu").Dot("RLock").Call(),
		jen.List(jen.Id("converterIface"), jen.Id("ok")).Op(":=").Id("converters").Index(jen.Id("name")),
		jen.Id("convertersMu").Dot("RUnlock").Call(),
		jen.If(jen.Op("!").Id("ok")).Block(
			jen.Return(jen.Id("zero"), jen.Qual("fmt", "Errorf").Call(
				jen.Lit("converter %s not registered"),
				jen.Id("name"),
			)),
		),
		jen.List(jen.Id("converter"), jen.Id("ok")).Op(":=").Id("converterIface").Assert(
			jen.Id("Converter").Types(jen.Id("From"), jen.Id("To")),
		),
		jen.If(jen.Op("!").Id("ok")).Block(
			jen.Return(jen.Id("zero"), jen.Qual("fmt", "Errorf").Call(
				jen.Lit("converter %s has wrong type"),
				jen.Id("name"),
			)),
		),
		jen.Return(jen.Id("converter").Call(jen.Id("value"))),
	)

	f.Line()

	// Generate init with default converters
	if len(cfg.DefaultConverters) > 0 {
		generateInit(f, cfg)
	}
}

// generateInit generates the init function with default converters
func generateInit(f *jen.File, cfg *config.Config) {
	initStatements := []jen.Code{}

	for _, conv := range cfg.DefaultConverters {
		initStatements = append(initStatements,
			jen.Id("RegisterConverter").Call(
				jen.Lit(conv.Name),
				jen.Id(conv.Function),
			),
		)
	}

	f.Func().Id("init").Params().Block(initStatements...)

	f.Line()

	// Built-in converter: TimeToJSString
	f.Comment("TimeToJSString converts time.Time to JavaScript ISO 8601 string")
	f.Func().Id("TimeToJSString").Params(
		jen.Id("t").Qual("time", "Time"),
	).Params(jen.String(), jen.Error()).Block(
		jen.Return(jen.Id("t").Dot("Format").Call(jen.Qual("time", "RFC3339")), jen.Nil()),
	)

	f.Line()
}
