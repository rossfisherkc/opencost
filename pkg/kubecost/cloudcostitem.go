package kubecost

import (
	"fmt"
	"strings"
	"time"

	"github.com/opencost/opencost/pkg/filter/matcher"
	"github.com/opencost/opencost/pkg/log"
)

// These contain some labels that can be used on Cloud cost
// item to get the corresponding cluster its associated.
const (
	AWSMatchLabel1     = "eks_cluster_name"
	AWSMatchLabel2     = "alpha_eksctl_io_cluster_name"
	AlibabaMatchLabel1 = "ack.aliyun.com"
	GCPMatchLabel1     = "goog-k8s-cluster-name"
)

type CloudCostItemLabels map[string]string

func (ccil CloudCostItemLabels) Clone() CloudCostItemLabels {
	result := make(map[string]string, len(ccil))
	for k, v := range ccil {
		result[k] = v
	}
	return result
}

func (ccil CloudCostItemLabels) Equal(that CloudCostItemLabels) bool {
	if len(ccil) != len(that) {
		return false
	}

	// Maps are of equal length, so if all keys are in both maps, we don't
	// have to check the keys of the other map.
	for k, v := range ccil {
		if tv, ok := that[k]; !ok || v != tv {
			return false
		}
	}

	return true
}

type CloudCostItemProperties struct {
	ProviderID  string              `json:"providerID,omitempty"`
	Provider    string              `json:"provider,omitempty"`
	WorkGroupID string              `json:"workGroupID,omitempty"`
	BillingID   string              `json:"billingID,omitempty"`
	Service     string              `json:"service,omitempty"`
	Category    string              `json:"category,omitempty"`
	Labels      CloudCostItemLabels `json:"labels,omitempty"`
}

func (ccip CloudCostItemProperties) Equal(that CloudCostItemProperties) bool {
	return ccip.ProviderID == that.ProviderID &&
		ccip.Provider == that.Provider &&
		ccip.WorkGroupID == that.WorkGroupID &&
		ccip.BillingID == that.BillingID &&
		ccip.Service == that.Service &&
		ccip.Category == that.Category &&
		ccip.Labels.Equal(that.Labels)
}

func (ccip CloudCostItemProperties) Clone() CloudCostItemProperties {
	return CloudCostItemProperties{
		ProviderID:  ccip.ProviderID,
		Provider:    ccip.Provider,
		WorkGroupID: ccip.WorkGroupID,
		BillingID:   ccip.BillingID,
		Service:     ccip.Service,
		Category:    ccip.Category,
		Labels:      ccip.Labels.Clone(),
	}
}

func (ccip CloudCostItemProperties) Key() string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s", ccip.Provider, ccip.BillingID, ccip.WorkGroupID, ccip.Category, ccip.Service, ccip.ProviderID)
}

func (ccip CloudCostItemProperties) MonitoringKey() string {
	return fmt.Sprintf("%s/%s", ccip.Provider, ccip.ProviderID)
}

// CloudCostItem represents a CUR line item, identifying a cloud resource and
// its cost over some period of time.
type CloudCostItem struct {
	Properties   CloudCostItemProperties `json:"properties"`
	IsKubernetes bool                    `json:"isKubernetes"`
	Window       Window                  `json:"window"`
	Cost         float64                 `json:"cost"`
	NetCost      float64                 `json:"netCost"`
}

// NewCloudCostItem instantiates a new CloudCostItem asset
func NewCloudCostItem(start, end time.Time, cciProperties CloudCostItemProperties, isKubernetes bool, cost, netcost float64) *CloudCostItem {
	return &CloudCostItem{
		Properties:   cciProperties,
		IsKubernetes: isKubernetes,
		Window:       NewWindow(&start, &end),
		Cost:         cost,
		NetCost:      netcost,
	}
}

func (cci *CloudCostItem) Clone() *CloudCostItem {
	return &CloudCostItem{
		Properties:   cci.Properties.Clone(),
		IsKubernetes: cci.IsKubernetes,
		Window:       cci.Window.Clone(),
		Cost:         cci.Cost,
		NetCost:      cci.NetCost,
	}
}

func (cci *CloudCostItem) Equal(that *CloudCostItem) bool {
	if that == nil {
		return false
	}

	return cci.Properties.Equal(that.Properties) &&
		cci.IsKubernetes == that.IsKubernetes &&
		cci.Window.Equal(that.Window) &&
		cci.Cost == that.Cost &&
		cci.NetCost == that.NetCost
}

func (cci *CloudCostItem) Key() string {
	return cci.Properties.Key()
}

func (cci *CloudCostItem) add(that *CloudCostItem) {
	if cci == nil {
		log.Warnf("cannot add to nil CloudCostItem")
		return
	}

	cci.Cost += that.Cost
	cci.NetCost += that.NetCost
	cci.Window = cci.Window.Expand(that.Window)
}

func (cci *CloudCostItem) MonitoringKey() string {
	return cci.Properties.MonitoringKey()
}

// Ony use compute resources to get Cluster names
func (cci *CloudCostItem) GetCluster() string {
	switch provider := cci.Properties.Provider; provider {
	case AWSProvider:
		return cci.GetAWSCluster()
	case AzureProvider:
		return cci.GetAzureCluster()
	case GCPProvider:
		return cci.GetGCPCluster()
	case AlibabaProvider:
		return cci.GetAlibabaCluster()
	default:
		log.Warnf("unsupported CloudCostItem found for a provider: %s", provider)
		return ""
	}
}

// Add any new ways of finding GCP cluster from Cloud cost Item
func (cci *CloudCostItem) GetGCPCluster() string {
	// currently from Cloud cost compute unable to get cluster name so returning empty
	return ""
}

// Add any new ways of finding AWS cluster from Cloud cost Item
func (cci *CloudCostItem) GetAWSCluster() string {
	if cci == nil {
		return ""
	}

	// This flag should be removed with filters in the compute query
	if cci.Properties.Provider != AWSProvider || cci.Properties.Category != ComputeCategory {
		return ""
	}
	// cn be either of these two labels to distinguish cluster name for a given providerID
	if val, ok := cci.Properties.Labels[AWSMatchLabel1]; ok {
		return val
	}
	if val, ok := cci.Properties.Labels[AWSMatchLabel2]; ok {
		return val
	}
	return ""
}

// Add any new ways of finding Azure cluster from Cloud cost Item
func (cci *CloudCostItem) GetAzureCluster() string {
	if cci == nil {
		return ""
	}

	// This flag should be removed with filters in the compute query
	if cci.Properties.Provider != AzureProvider || cci.Properties.Category != ComputeCategory {
		return ""
	}

	providerIDSplit := strings.Split(cci.Properties.ProviderID, "/")
	// ensure this is actually returnable before return
	if len(providerIDSplit) < 6 {
		return ""
	}
	return strings.Split(cci.Properties.ProviderID, "/")[6]
}

// Add any new ways of finding Alibaba cluster from Cloud cost Item
func (cci *CloudCostItem) GetAlibabaCluster() string {
	if cci == nil {
		return ""
	}

	// This flag should be removed with filters in the compute query
	if cci.Properties.Provider != AlibabaProvider || cci.Properties.Category != ComputeCategory {
		return ""
	}
	if val, ok := cci.Properties.Labels[AlibabaMatchLabel1]; ok {
		return val
	}
	return ""
}

type CloudCostItemSet struct {
	CloudCostItems map[string]*CloudCostItem `json:"items"`
	Window         Window                    `json:"window"`
	Integration    string                    `json:"-"`
}

// NewAssetSet instantiates a new AssetSet and, optionally, inserts
// the given list of Assets
func NewCloudCostItemSet(start, end time.Time, cloudCostItems ...*CloudCostItem) *CloudCostItemSet {
	ccis := &CloudCostItemSet{
		CloudCostItems: map[string]*CloudCostItem{},
		Window:         NewWindow(&start, &end),
	}

	for _, cci := range cloudCostItems {
		ccis.Insert(cci)
	}

	return ccis
}

func (ccis *CloudCostItemSet) Accumulate(that *CloudCostItemSet) (*CloudCostItemSet, error) {
	if ccis.IsEmpty() {
		return that.Clone(), nil
	}

	if that.IsEmpty() {
		return ccis.Clone(), nil
	}
	// Set start, end to min(start), max(end)
	start := ccis.Window.Start()
	end := ccis.Window.End()
	if that.Window.Start().Before(*start) {
		start = that.Window.Start()
	}
	if that.Window.End().After(*end) {
		end = that.Window.End()
	}

	acc := NewCloudCostItemSet(*start, *end)

	for _, cci := range ccis.CloudCostItems {
		err := acc.Insert(cci)
		if err != nil {
			return nil, err
		}
	}

	for _, cci := range that.CloudCostItems {
		err := acc.Insert(cci)
		if err != nil {
			return nil, err
		}
	}
	return acc, nil
}

func (ccis *CloudCostItemSet) Equal(that *CloudCostItemSet) bool {
	if ccis.Integration != that.Integration {
		return false
	}

	if !ccis.Window.Equal(that.Window) {
		return false
	}

	if len(ccis.CloudCostItems) != len(that.CloudCostItems) {
		return false
	}

	for k, cci := range ccis.CloudCostItems {
		tcci, ok := that.CloudCostItems[k]
		if !ok {
			return false
		}
		if !cci.Equal(tcci) {
			return false
		}
	}

	return true
}

func (ccis *CloudCostItemSet) Filter(filters matcher.Matcher[*CloudCostItem]) *CloudCostItemSet {
	if ccis == nil {
		return nil
	}

	if filters == nil {
		return ccis.Clone()
	}

	result := NewCloudCostItemSet(*ccis.Window.start, *ccis.Window.end)

	for _, cci := range ccis.CloudCostItems {
		if filters.Matches(cci) {
			result.Insert(cci.Clone())
		}
	}

	return result
}

func (ccis *CloudCostItemSet) Insert(that *CloudCostItem) error {
	if ccis == nil {
		return fmt.Errorf("cannot insert into nil CloudCostItemSet")
	}

	if that == nil {
		return fmt.Errorf("cannot insert nil CloudCostItem into CloudCostItemSet")
	}

	if ccis.CloudCostItems == nil {
		ccis.CloudCostItems = map[string]*CloudCostItem{}
	}

	// Add the given CloudCostItem to the existing entry, if there is one;
	// otherwise just set directly into allocations
	if _, ok := ccis.CloudCostItems[that.Key()]; !ok {
		ccis.CloudCostItems[that.Key()] = that.Clone()
	} else {
		ccis.CloudCostItems[that.Key()].add(that)
	}

	return nil
}

func (ccis *CloudCostItemSet) Clone() *CloudCostItemSet {
	items := make(map[string]*CloudCostItem, len(ccis.CloudCostItems))
	for k, v := range ccis.CloudCostItems {
		items[k] = v.Clone()
	}

	return &CloudCostItemSet{
		CloudCostItems: items,
		Integration:    ccis.Integration,
		Window:         ccis.Window.Clone(),
	}
}

func (ccis *CloudCostItemSet) IsEmpty() bool {
	if ccis == nil {
		return true
	}

	if len(ccis.CloudCostItems) == 0 {
		return true
	}

	return false
}

func (ccis *CloudCostItemSet) Length() int {
	if ccis == nil {
		return 0
	}
	return len(ccis.CloudCostItems)
}

func (ccis *CloudCostItemSet) GetWindow() Window {
	return ccis.Window
}

func (ccis *CloudCostItemSet) Merge(that *CloudCostItemSet) (*CloudCostItemSet, error) {
	if ccis == nil {
		return nil, fmt.Errorf("cannot merge nil CloudCostItemSets")
	}

	if that.IsEmpty() {
		return ccis.Clone(), nil
	}

	if !ccis.Window.Equal(that.Window) {
		return nil, fmt.Errorf("cannot merge CloudCostItemSets with different windows")
	}

	start, end := *ccis.Window.Start(), *ccis.Window.End()
	result := NewCloudCostItemSet(start, end)

	for _, cci := range ccis.CloudCostItems {
		result.Insert(cci)
	}

	for _, cci := range that.CloudCostItems {
		result.Insert(cci)
	}

	return result, nil
}

type CloudCostItemSetRange struct {
	CloudCostItemSets []*CloudCostItemSet `json:"sets"`
	Window            Window              `json:"window"`
}

// NewCloudCostItemSetRange create a CloudCostItemSetRange containing CloudCostItemSets with windows of equal duration
// the duration between start and end must be divisible by the window duration argument
func NewCloudCostItemSetRange(start time.Time, end time.Time, window time.Duration, integration string) (*CloudCostItemSetRange, error) {
	windows, err := GetWindows(start, end, window)
	if err != nil {
		return nil, err
	}

	// Build slice of CloudCostItemSet to cover the range
	cloudCostItemSets := make([]*CloudCostItemSet, len(windows))
	for i, w := range windows {
		ccis := NewCloudCostItemSet(*w.Start(), *w.End())
		ccis.Integration = integration
		cloudCostItemSets[i] = ccis
	}
	return &CloudCostItemSetRange{
		Window:            NewWindow(&start, &end),
		CloudCostItemSets: cloudCostItemSets,
	}, nil
}

func (ccisr *CloudCostItemSetRange) Clone() *CloudCostItemSetRange {
	ccisSlice := make([]*CloudCostItemSet, len(ccisr.CloudCostItemSets))
	for i, ccis := range ccisr.CloudCostItemSets {
		ccisSlice[i] = ccis.Clone()
	}
	return &CloudCostItemSetRange{
		Window:            ccisr.Window.Clone(),
		CloudCostItemSets: ccisSlice,
	}
}

// Accumulate sums each CloudCostItemSet in the given range, returning a single cumulative
// CloudCostItemSet for the entire range.
func (ccisr *CloudCostItemSetRange) Accumulate() (*CloudCostItemSet, error) {
	var cloudCostItemSet *CloudCostItemSet
	var err error

	for _, ccis := range ccisr.CloudCostItemSets {
		cloudCostItemSet, err = cloudCostItemSet.Accumulate(ccis)
		if err != nil {
			return nil, err
		}
	}

	return cloudCostItemSet, nil
}

// LoadCloudCostItem loads CloudCostItems into existing CloudCostItemSets of the CloudCostItemSetRange.
// This function service to aggregate and distribute costs over predefined windows
// are accumulated here so that the resulting CloudCostItem with the 1d window has the correct price for the entire day.
// If all or a portion of the window of the CloudCostItem is outside of the windows of the existing CloudCostItemSets,
// that portion of the CloudCostItem's cost will not be inserted
func (ccisr *CloudCostItemSetRange) LoadCloudCostItem(cloudCostItem *CloudCostItem) {
	window := cloudCostItem.Window
	if window.IsOpen() {
		log.Errorf("CloudCostItemSetRange: LoadCloudCostItem: invalid window %s", window.String())
		return
	}

	totalPct := 0.0

	// Distribute cost of the current item across one or more CloudCostItems in
	// across each relevant CloudCostItemSet. Stop when the end of the current
	// block reaches the item's end time or the end of the range.
	for _, ccis := range ccisr.CloudCostItemSets {
		setWindow := ccis.Window

		// get percent of item window contained in set window
		pct := setWindow.GetPercentInWindow(window)
		if pct == 0 {
			continue
		}

		cci := cloudCostItem
		// If the current set Window only contains a portion of the CloudCostItem Window, insert costs relative to that portion
		if pct < 1.0 {
			cci = &CloudCostItem{
				Properties:   cloudCostItem.Properties,
				IsKubernetes: cloudCostItem.IsKubernetes,
				Window:       window.Contract(setWindow),
				Cost:         cloudCostItem.Cost * pct,
				NetCost:      cloudCostItem.NetCost * pct,
			}
		}

		err := ccis.Insert(cci)
		if err != nil {
			log.Errorf("CloudCostItemSetRange: LoadCloudCostItem: failed to load CloudCostItem with key %s and window %s: %s", cci.Key(), ccis.GetWindow().String(), err.Error())
		}

		// If all cost has been inserted then finish
		totalPct += pct
		if totalPct >= 1.0 {
			return
		}
	}
}
