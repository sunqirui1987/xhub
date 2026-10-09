# Model catalog, rates, and normalized usage

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

Embedded public data, supplier contributions, optional feeds, and local overrides form the catalog. routes.json describes route wiring, not universal provider support. CostAt selects rates using model, normalized usage, and call time; holiday.go supports pricing windows.
Rate.USD is already expressed per base unit. Do not divide by UnitSize again. Rates can measure tokens, pictures, seconds, or queries; an explicit zero is different from missing pricing.
Usage normalization preserves missing, null, and zero semantics. OpenAI nested cached input is generally a subset of prompt usage; Anthropic top-level cache fields contribute to total input. Applied quantities and prices are retained as billing snapshots so later catalog changes do not rewrite history.

## Subdirectories and collaboration

| Directory | Responsibility |
| --- | --- |
| [price_binding_test.go](price_binding_test.go) | TestExplicitSellerPriceBindingSurvivesRefreshAndRejectsOtherSource: explicit seller pricing survives refresh and rejects another source. |
| [publicdata](publicdata/readme.md) | Embedded public catalog resources |

## Source responsibilities and entry points

### classify.go

Exported types: `Route`, `AuthClass`.

- [`func Load() []Route`](classify.go) — Load reads the embedded routes.json. A parse failure returns nil, and the caller treats that as an empty catalog.
- [`func AuthOf(path string) AuthClass`](classify.go) — AuthOf chooses the identity class from the path prefix. A mixed prefix wins over an inference prefix.
- [`func IsMixedPath(path string) bool`](classify.go) — IsMixedPath reports the catalog paths that were removed from the product and are refused rather than served. /v1/skills is deliberately absent: it is a real route again, serving the organization's skills. A path leaves this list when a dedicated handler takes it over, or the refusal would answer before the handler ever ran.
- [`func IsPublicPath(method, path string) bool`](classify.go) — IsPublicPath reports a GET or HEAD that needs no identity. Only public pages, well-known URLs, and the model-hub entry qualify.
- [`func IsDataPlanePath(path string) bool`](classify.go) — IsDataPlanePath reports a path that enters the inference data plane, including removed mixed prefixes.
- [`func IsLLMPrefix(path string) bool`](classify.go) — IsLLMPrefix reports a path prefix for OpenAI, Anthropic, Gemini, and compatible endpoints.
- [`func HasPrefix(path, prefix string) bool`](classify.go) — HasPrefix compares path prefixes. A trailing slash is ignored so /v1/chat does not match /v1/chatcompletions.
- [`func PublicBody(path string) any`](classify.go) — PublicBody is the fixed response for a public GET. The price map and create fields come from embedded JSON. Everything else is an empty list.
- [`func PathMatch(pat, path string) bool`](classify.go) — PathMatch reports whether a catalog template covers a concrete path. {name} matches one segment. :path and {x:path} match the rest.
- [`func Split(p string) []string`](classify.go) — Split splits a path on slashes. An empty path returns nil, not a slice containing an empty string.

### contribute.go

Exported types: `Row`, `ProviderField`, `ProviderRow`.

- [`func Contribute(row Row)`](contribute.go) — Contribute inserts a model into the price map and the per-provider index. The official id is remembered so a bill for that id finds the same rates.
- [`func ContributeProvider(p ProviderRow)`](contribute.go) — ContributeProvider remembers a dropdown entry. PublicBody appends it when the embedded list does not already use that provider name.

### cost.go

- [`func Cost(model string, prompt, completion int) (total, input, output float64, ok bool)`](cost.go) — Cost returns the dollar total, input cost, and output cost for a token count. An unknown model returns ok false so the caller does not record a free call.
- [`func Format(v float64) string`](cost.go) — Format renders a dollar amount as a decimal string without trailing zeros.

### cost_at.go

Exported types: `Usage`, `Charge`, `AppliedRate`, `PriceSnapshot`.

- [`func WindowAt(t time.Time) string`](cost_at.go) — WindowAt reports which billing window a call starting at t falls in. Callers that price from their own rate table need the window on its own.
- [`func DecodeRates(raw any) ([]Rate, bool)`](cost_at.go) — DecodeRates reads a rate table from the shape it arrives in over the wire - either the embedded catalog's own structure or a deployment's JSON. One decoder for both keeps an operator-typed table and a generated one priced the same way.
- [`func RatesFromFlat(read func(string) (float64, bool)) []Rate`](cost_at.go) — RatesFromFlat builds a rate table from the flat price fields a deployment or an older price row carries. The flat form cannot say what it does not have a field for - a picture price per resolution, an input price per modality, a second of 4K video against a second of 1080p - but the four fields it does have must be read by the same biller as everything else. Routing them through here rather than through a second arithmetic path is what keeps a peak rate, a cache read and a per-image price from being dropped on the way to the usage row. read is called once per known field and returns ok false for one the source does not carry; a field present as zero is a real price of zero.
- [`func CostFromFlatOrRates(read func(string) (float64, bool), usage Usage, startedAt time.Time) (Charge, bool)`](cost_at.go) — CostFromFlatOrRates prices a call against flat price fields, read through the callback, or reports that the source carries no price at all. It exists so a caller holding the flat form does not have to build a table and remember to keep the two paths in step. startedAt（time.Time）：调用开始的时刻。
- [`func NormalizeUsage(usage map[string]any) Usage`](cost_at.go) — NormalizeUsage reads the quantities the gateway bills on out of the usage object an upstream returned, resolving the field names providers spell differently into one set of counts. It exists because the providers do not agree on what "input tokens" contains, and the disagreement runs through several field names at once. OpenAI      prompt_tokens is the whole prompt; the cached part is repeated under prompt_tokens_details.cached_tokens as a subset of it. Anthropic   input_tokens is *only* the part that missed the cache, and cache_read_input_tokens is a separate count beside it. Responses   input_tokens is the whole prompt, with the cached part nested under input_tokens_details the way OpenAI spells it. So the discriminator is *where the cache count lives*, not the field name of the prompt c
- [`func CostFromRates(rates []Rate, usage Usage, startedAt time.Time) (Charge, bool)`](cost_at.go) — CostFromRates prices a call against a rate table handed in by the caller rather than one looked up from the catalog. A deployment that typed its own rates goes through here, so both sources are billed by the same rules.
- [`func Snapshot(c Charge) string`](cost_at.go) — Snapshot renders the rates a charge used as the JSON stored beside the usage row. A caller with no charge gets an empty string, which is how a usage row records that this call was never priced.
- [`func SnapshotUsage(c Charge, u Usage, priced bool) string`](cost_at.go) — Records measured async usage, the applied rates, and whether the result could actually be priced. A missing video band remains unpriced even when the provider reported tokens.
- [`func CostAt(model string, usage Usage, startedAt time.Time) (Charge, bool)`](cost_at.go) — CostAt prices one call at the instant it started. The instant decides the window, and the call's own facts decide the variant. Both are needed: a cached prompt on a peak-hour call at a window-priced model is four different rates away from an uncached prompt on an offpeak one. It returns ok false when the model is unknown or when none of the quantities the call reported can be priced. A caller that gets false must record the call as unpriced - not as free - because the two are different facts. startedAt（time.Time）：调用开始的时刻。时段只由它决定，不用结束时刻。

### embed.go

- [`func Embedded(name string, raw []byte) []byte`](embed.go) — Embedded returns one embedded JSON document. The first valid document is logged once. A document that is not JSON is logged as an error.

### feed.go

- [`func BuildPriceDocument(raw []byte) (PriceDocument, error)`](feed.go) — BuildPriceDocument converts a market feed response into the price catalog document. An empty feed returns an error rather than an empty catalog, so a failed reload cannot wipe the prices in use.
- [`func FetchMarket(ctx context.Context, url string) (PriceDocument, error)`](feed.go) — FetchMarket reads the market feed. It is the runtime counterpart of the build-time generator, and both go through BuildPriceDocument.
- [`func ReloadFromMarket(ctx context.Context, url string) (int, error)`](feed.go) — ReloadFromMarket fetches the market feed and swaps it in as the live price catalog. A failed fetch leaves the prices in use untouched and reports why.

### holiday.go

- [`func IsPeakHour(t time.Time) bool`](holiday.go) — IsPeakHour reports whether t falls inside a peak billing window. Peak is defined as Mon–Fri 09:00–12:00 and 14:00–18:00 Beijing time (Asia/Shanghai), excluding Chinese public holidays and including make-up work days that fall on a weekend (调休补班). The rule mirrors DeepSeek's published pricing; any other provider with a window-aware rate table uses the same function because the window dimension is a property of the rate, not of the provider. Boundary treatment: 09:00 and 14:00 are peak; 12:00 and 18:00 are not. This matches the convention of half-open intervals [start, end).

### model_cost.go

- [`func Raw() any`](model_cost.go) — Raw returns the raw JSON object of the built-in price map.
- [`func TokenRates(model string) (input, output float64, ok bool)`](model_cost.go) — TokenRates returns the per-token input and output prices for one model in the built-in price map. The name is matched as stored, and if that misses, the provider prefix before the first slash is dropped. A missing name or a row with no per-token prices returns ok false.
- [`func CostMap() map[string]map[string]any`](model_cost.go) — CostMap turns the price map into a model-name to fields map. An entry that is not an object is dropped.
- [`func Count() int`](model_cost.go) — Count is the number of models in the price map.
- [`func LoadedAt() string`](model_cost.go) — LoadedAt is the UTC time, in RFC3339, when the process loaded this built-in price map.
- [`func MarkReloaded() int`](model_cost.go) — MarkReloaded records that the price catalog was refreshed and returns the number of models now in it. The rows themselves come from ReloadFromMarket; this only stamps the time the console displays.
- [`func EnvForced() bool`](model_cost.go) — EnvForced reports that LITELLM_LOCAL_MODEL_COST_MAP is set to true, ignoring case. When it is true the gateway uses only the built-in map and does not try a remote price source.
- [`func ProviderModels(provider string) []string`](model_cost.go) — ProviderModels returns the model ids for one provider in the built-in map. An unknown provider returns nil.
- [`func KnownProvider(name string) bool`](model_cost.go) — KnownProvider reports that this prefix is a supplier name rather than an organization id. While expanding a wildcard, only a known provider prefix is stripped and then joined with the caller's prefix.

### price_write.go

- [`func BaselineModel(id string) (map[string]any, bool)`](price_write.go) — BaselineModel returns the embedded row for an id, or ok false when the generated catalog does not contain that model.
- [`func ModelRow(id string) (map[string]any, bool)`](price_write.go) — ModelRow returns the live row for an id, which is the baseline row or the override that replaced it. A missing id returns ok false.
- [`func IsBaseline(id string) bool`](price_write.go) — IsBaseline reports whether this id comes from the generated catalog rather than from an operator's entry.
- [`func SetModel(id string, row map[string]any)`](price_write.go) — SetModel writes one row into the live price map and indexes it under its provider. An existing row is replaced, so this is also how an edit lands.
- [`func RemoveModel(id string) bool`](price_write.go) — RemoveModel drops one row from the live map and its provider index. The embedded baseline is untouched, so the row returns after a restart unless the caller also deleted the stored override.
- [`func ApplyDocument(doc PriceDocument) (int, error)`](price_write.go) — ApplyDocument replaces the live catalog with a freshly generated one. The embedded baseline is untouched, so an operator's overrides and deletions still apply on top of it and a later restart falls back to the same rows. A document with no models is refused: a failed fetch must not empty the catalog the gateway is billing from.
- [`func SetProvider(slug string, row map[string]any)`](price_write.go) — SetProvider writes one supplier into the add-model dropdown, replacing an entry that already uses the same slug.
- [`func RemoveProvider(slug string) bool`](price_write.go) — RemoveProvider drops one supplier from the dropdown.
- [`func IsBaselineProvider(slug string) bool`](price_write.go) — IsBaselineProvider reports whether this supplier comes from the generated catalog rather than from an operator's entry.

### pricedata.go

Exported types: `PriceDocument`.

- [`func Providers() []map[string]any`](pricedata.go) — Providers 返回内嵌目录里的供应商列表，供添加模型时的下拉框使用，含凭据字段。
- [`func PriceSource() string`](pricedata.go) — PriceSource 返回生成这份内嵌目录时用的来源地址。
- [`func PriceGeneratedAt() string`](pricedata.go) — PriceGeneratedAt 返回这份内嵌目录的生成时间。

### rates.go

Exported types: `Rate`.

Internal implementation and protocol boundaries:  [rates.go](rates.go)。

Resources and persistence definitions: [routes.json](routes.json).

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [price_binding_test.go](price_binding_test.go) | TestExplicitSellerPriceBindingSurvivesRefreshAndRejectsOtherSource: explicit seller pricing survives refresh and rejects another source. |
| [classify_regression_test.go](classify_regression_test.go) | `TestProviderResourceAliasesRequireInferenceIdentity` |
| [cost_at_test.go](cost_at_test.go) | `TestPeakCostsTwiceOffpeak`, `TestUnwindowedModelCostsTheSameAtBothInstants`, `TestPeakNeverFallsBackToTheCheapRate`, `TestCacheReadIsNotBilledAtTheInputRate`, `TestCachedVariantOfInputSideIsHonoured`, `TestCachedTokensAreNotDoubleBilled`, `TestCacheHitAbovePromptCountIsClamped`, `TestModelWithoutACacheRateBillsTheWholePromptAsInput`, `TestPerSecondModelIsBilledBySeconds`, `TestPerPictureModelIsBilledByPictures`, `TestMeasuredSidesDoNotBleedIntoEachOther`, `TestUnknownModelIsNotPriced`, `TestZeroQuantityIsNotPriced`, `TestAppliedRatesAreRecordedForTheSnapshot`, `TestUnitSizeIsAppliedOnce`, `TestRateTableIsReadFromJSONShape`, `TestRowWithoutRatesIsBilledFromItsFlatFields`, `TestFlatPeakFieldIsHonoured`, `TestFlatNonTokenMeasuresAreBilled`, `TestFlatCacheReadIsNotCountedTwice`, `TestVariantOnlySideFallsBackAndSaysSo`, `TestAnUnqualifiedRateIsNotDisplacedByTheFallback`, `TestACacheReadIsNeverBilledAtTheInputPrice`, `TestASearchPriceIsBilledAsAQuery`, `TestAQueryWithoutAPriceIsNotBillable`, `TestNormalizeUsageResolvesTheProviderShapes`, `TestNormalizeUsageReadsTheNonTokenMeasures`, `TestNormalizeUsageOfNothingIsZero`, `TestRatesFromFlatKeepsEveryMeasure`, `TestRatesFromFlatMakesOnePriceApplyEveryHour`, `TestRatesFromFlatKeepsThePeakLadder`, `TestRatesFromFlatOfNothingIsEmpty`, `TestAPrefixedNameResolvesItsCatalogAlias`, `TestAnUnpricedGatewayNameDoesNotHideTheCatalogRow`, `TestPriceKeysKeepTheirPreferenceOrder` |
| [feed_test.go](feed_test.go) | `TestPlainTokenModelKeepsBothSides`, `TestVariantPricedVideoModelGetsABillableRate`, `TestPicturePricedInputIsNotATokenRate`, `TestVariantPricedImageModelGetsABillableRate`, `TestVariantPerOutputTokenIsBillable`, `TestPeakOffPeakVariantsStillBill`, `TestReasoningVariantsStillBill`, `TestModelWithNoPricingAtAllStaysUnpriced`, `TestGenuineZeroIsKept`, `TestPeakOffPeakRatesHaveWindowDimension`, `TestPlainModelRatesAllWindow`, `TestEmptyFeedIsRefused` |
| [holiday_test.go](holiday_test.go) | `TestIsPeakHour_WeekdayMorning`, `TestIsPeakHour_WeekdayAfternoon`, `TestIsPeakHour_WeekdaySaturdayIsOffpeak`, `TestIsPeakHour_Noon`, `TestIsPeakHour_MorningStart`, `TestIsPeakHour_AfternoonEnd`, `TestIsPeakHour_LunchBreak`, `TestIsPeakHour_PublicHoliday`, `TestIsPeakHour_HolidayWeekday`, `TestIsPeakHour_MakeUpWorkSaturday`, `TestIsPeakHour_SpringFestivalMakeUp`, `TestIsPeakHour_NationalDayHoliday`, `TestIsPeakHour_NationalDayMakeUp`, `TestParseHolidays` |
| [usage_regression_test.go](usage_regression_test.go) | `TestNormalizeUsagePreservesExplicitZeroOverAliases` |

```bash
go test ./internal/catalog -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
