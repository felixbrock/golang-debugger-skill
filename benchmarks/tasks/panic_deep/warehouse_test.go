package warehouse

import (
	"reflect"
	"testing"
)

func demoCatalog() *Catalog {
	c := NewCatalog()
	c.Add(Item{SKU: "CBL-USB-2M", Name: "USB cable 2m", Stock: 120})
	c.Add(Item{SKU: "CBL-USB-05M", Name: "USB cable 0.5m", Stock: 44})
	c.Add(Item{SKU: "HUB-7P", Name: "7-port hub", Stock: 9})
	c.Add(Item{SKU: "SSD-1T", Name: "1TB SSD", Stock: 0, Backordered: true})
	c.Add(Item{SKU: "SSD-2T", Name: "2TB SSD", Stock: 17})
	c.Add(Item{SKU: "RAM-32G", Name: "32GB DIMM", Stock: 25})
	c.Add(Item{SKU: "PSU-650", Name: "650W PSU", Stock: 6})
	c.Add(Item{SKU: "GPU-Q4", Name: "Quadro Q4", Stock: 0, Backordered: true})
	return c
}

func TestFillsAnOrderWithBackorderedItems(t *testing.T) {
	c := demoCatalog()
	picks, err := Resolve(c, []OrderLine{
		{SKU: "hub-7p", Qty: 1},
		{SKU: "ssd-1t", Qty: 2},
		{SKU: "CBL USB 2M", Qty: 3},
		{SKU: "gpu-q4", Qty: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := Fill(c, picks)
	want := Shipment{
		SKUs:    []string{"HUB-7P", "SSD-1T", "CBL-USB-2M", "GPU-Q4"},
		Partial: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Fill() = %+v, want %+v", got, want)
	}
}
