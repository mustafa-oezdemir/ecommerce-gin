package models

import "testing"

func TestProductGalleryImagesPlacesCoverFirst(t *testing.T) {
	product := Product{
		ImageFilename: "cover.jpg",
		Images: []ProductImage{
			{ID: 3, Filename: "third.jpg", Position: 2},
			{ID: 1, Filename: "first.jpg", Position: 0},
			{ID: 2, Filename: "cover.jpg", Position: 1},
		},
	}

	images := product.GalleryImages()
	if len(images) != 3 || images[0].Filename != "cover.jpg" || images[1].Filename != "first.jpg" || images[2].Filename != "third.jpg" {
		t.Fatalf("unexpected gallery order: %#v", images)
	}
	if product.Images[0].Filename != "third.jpg" {
		t.Fatal("GalleryImages changed the product association in place")
	}
}

func TestProductGalleryImagesSupportsLegacyCover(t *testing.T) {
	images := (Product{ImageFilename: "legacy.png"}).GalleryImages()
	if len(images) != 1 || images[0].Filename != "legacy.png" {
		t.Fatalf("legacy cover was not returned: %#v", images)
	}
}
