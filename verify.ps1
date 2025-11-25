$baseUrl = "http://localhost:8080"

# 1. Create a Source
Write-Host "Creating Source..."
$sourceBody = @{
    name = "Test Source"
    type = "url"
    location = "http://example.com"
} | ConvertTo-Json
$source = Invoke-RestMethod -Uri "$baseUrl/sources" -Method Post -Body $sourceBody -ContentType "application/json"
Write-Host "Source Created: $($source.id)"

# 2. List Sources
Write-Host "Listing Sources..."
$sources = Invoke-RestMethod -Uri "$baseUrl/sources" -Method Get
Write-Host "Sources: $($sources.Count)"

# 3. List Galleries (should have one auto-created)
Write-Host "Listing Galleries..."
$galleries = Invoke-RestMethod -Uri "$baseUrl/galleries" -Method Get
Write-Host "Galleries: $($galleries.Count)"
$galleryId = $galleries[0].id
Write-Host "Using Gallery ID: $galleryId"

# 4. Add Image to Gallery
# Using a reliable image URL
$imageUrl = "https://www.google.com/images/branding/googlelogo/1x/googlelogo_color_272x92dp.png"
Write-Host "Adding Image to Gallery..."
$imageBody = @{
    url = $imageUrl
    filename = "test_image.png"
} | ConvertTo-Json
try {
    $image = Invoke-RestMethod -Uri "$baseUrl/galleries/$galleryId/images" -Method Post -Body $imageBody -ContentType "application/json"
    Write-Host "Image Added: $($image.id)"
} catch {
    Write-Host "Failed to add image: $_"
    exit 1
}

# 5. Verify Files Exist
$uploadsDir = "uploads"
if (Test-Path "$uploadsDir/test_image.png") {
    Write-Host "Image file exists."
} else {
    Write-Error "Image file missing!"
}
if (Test-Path "$uploadsDir/test_image_thumb.jpg") {
    Write-Host "Thumbnail file exists."
} else {
    Write-Error "Thumbnail file missing!"
}

# 6. Request Image via API
Write-Host "Requesting Image via API..."
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/images/test_image.png" -Method Get
    if ($response.StatusCode -eq 200) {
        Write-Host "Image served successfully."
    }
} catch {
    Write-Error "Failed to serve image: $_"
}

Write-Host "Verification Complete."
