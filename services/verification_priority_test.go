package services

import (
	"testing"
	"time"
)

func TestPrioritizeImageRecoveryTasks(t *testing.T) {
	now := time.Now()

	// Setup galleries:
	// Gal 1: Favorite gallery (created 10 days ago)
	// Gal 2: Oldest non-fav gallery (created 20 days ago)
	// Gal 3: Middle non-fav gallery (created 15 days ago)
	// Gal 4: Newest non-fav gallery (created 5 days ago)
	galleryCreatedAt := map[uint]time.Time{
		1: now.Add(-10 * 24 * time.Hour),
		2: now.Add(-20 * 24 * time.Hour), // oldest non-fav
		3: now.Add(-15 * 24 * time.Hour), // middle non-fav
		4: now.Add(-5 * 24 * time.Hour),  // newest non-fav
	}

	favGalleryIDs := map[uint]bool{
		1: true,
	}

	tasks := []ImageRecoveryTask{
		// Non-fav in middle non-fav gallery (Gal 3)
		{ID: 10, IsFavorite: false, GalleryIDs: []uint{3}},
		// Favorite in Gal 2 (itself is a favorite image -> Tier 1)
		{ID: 11, IsFavorite: true, GalleryIDs: []uint{2}},
		// Non-fav in fav gallery (Gal 1 -> Tier 2)
		{ID: 12, IsFavorite: false, GalleryIDs: []uint{1}},
		// Non-fav in oldest non-fav gallery (Gal 2 -> Tier 3)
		{ID: 13, IsFavorite: false, GalleryIDs: []uint{2}},
		// Favorite with no gallery -> Tier 1
		{ID: 14, IsFavorite: true, GalleryIDs: nil},
		// Non-fav in newest non-fav gallery (Gal 4 -> Tier 3)
		{ID: 15, IsFavorite: false, GalleryIDs: []uint{4}},
		// Orphan non-fav -> Tier 3 end
		{ID: 16, IsFavorite: false, GalleryIDs: nil},
		// Another non-fav in fav gallery (Gal 1 -> Tier 2)
		{ID: 17, IsFavorite: false, GalleryIDs: []uint{1}},
	}

	prioritized := PrioritizeImageRecoveryTasks(tasks, favGalleryIDs, galleryCreatedAt)

	if len(prioritized) != len(tasks) {
		t.Fatalf("expected %d tasks, got %d", len(tasks), len(prioritized))
	}

	// Tier 1: Favorites first (IDs: 11, 14)
	if !prioritized[0].IsFavorite || (prioritized[0].ID != 11 && prioritized[0].ID != 14) {
		t.Errorf("expected favorite at index 0, got task ID %d", prioritized[0].ID)
	}
	if !prioritized[1].IsFavorite || (prioritized[1].ID != 11 && prioritized[1].ID != 14) {
		t.Errorf("expected favorite at index 1, got task ID %d", prioritized[1].ID)
	}

	// Tier 2: Images from favorite galleries (IDs: 12, 17)
	if prioritized[2].ID != 12 || prioritized[3].ID != 17 {
		t.Errorf("expected favorite gallery images at index 2 and 3, got IDs %d, %d", prioritized[2].ID, prioritized[3].ID)
	}

	// Tier 3: Alternating between newest and oldest galleries
	// Gal 4 (newest) -> ID 15
	// Gal 2 (oldest) -> ID 13
	// Gal 3 (middle remaining) -> ID 10
	if prioritized[4].ID != 15 {
		t.Errorf("expected newest gallery image (ID 15) at index 4, got ID %d", prioritized[4].ID)
	}
	if prioritized[5].ID != 13 {
		t.Errorf("expected oldest gallery image (ID 13) at index 5, got ID %d", prioritized[5].ID)
	}
	if prioritized[6].ID != 10 {
		t.Errorf("expected remaining gallery image (ID 10) at index 6, got ID %d", prioritized[6].ID)
	}

	// Orphan task at the end (ID: 16)
	if prioritized[7].ID != 16 {
		t.Errorf("expected orphan image (ID 16) at index 7, got ID %d", prioritized[7].ID)
	}
}

func TestSortVideoRecoveryTasks(t *testing.T) {
	tasks := []VideoRecoveryTask{
		{ID: 1, Title: "Large", SizeMB: 500.5},
		{ID: 2, Title: "Small", SizeMB: 12.3},
		{ID: 3, Title: "Medium", SizeMB: 150.0},
		{ID: 4, Title: "Zero/Unknown", SizeMB: 0.0},
		{ID: 5, Title: "Small Tie High ID", SizeMB: 12.3},
	}

	SortVideoRecoveryTasks(tasks)

	expectedOrder := []uint{4, 2, 5, 3, 1}
	for i, expectedID := range expectedOrder {
		if tasks[i].ID != expectedID {
			t.Errorf("at index %d, expected video ID %d (SizeMB: %.1f), got ID %d (SizeMB: %.1f)",
				i, expectedID, tasks[i].SizeMB, tasks[i].ID, tasks[i].SizeMB)
		}
	}
}
