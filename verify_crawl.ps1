$baseUrl = "http://localhost:8080"

# 1. Create a Source for Crawling
Write-Host "Creating Source for Crawling..."
$sourceBody = @{
    name = "Crawl Test Source"
    type = "url"
    location = "http://localhost:8081/test_page.html"
} | ConvertTo-Json
$source = Invoke-RestMethod -Uri "$baseUrl/sources" -Method Post -Body $sourceBody -ContentType "application/json"
$sourceId = $source.id
Write-Host "Source Created: $sourceId"

# 2. Trigger Crawl
Write-Host "Triggering Crawl..."
Invoke-RestMethod -Uri "$baseUrl/sources/$sourceId/crawl" -Method Post
Write-Host "Crawl Triggered."

# 3. Wait for Crawl to Finish (poll status)
Write-Host "Waiting for crawl..."
for ($i=0; $i -lt 10; $i++) {
    Start-Sleep -Seconds 2
    $sources = Invoke-RestMethod -Uri "$baseUrl/sources" -Method Get
    $s = $sources | Where-Object { $_.id -eq $sourceId }
    Write-Host "Status: $($s.status)"
    if ($s.status -eq "idle" -and $s.last_checked_at -ne $null) {
        break
    }
}

# 4. Verify Images
Write-Host "Verifying Images..."
$galleries = Invoke-RestMethod -Uri "$baseUrl/galleries" -Method Get
# Filter by the source ID we just created. 
# Note: The gallery API response includes source_id.
$gallery = $galleries | Where-Object { $_.source_id -eq $sourceId }

if ($gallery -and $gallery.images.Count -gt 0) {
    Write-Host "Success: Found $($gallery.images.Count) images in gallery for source $sourceId."
} else {
    Write-Error "Failure: No images found for source $sourceId."
}
