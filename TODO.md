IMPORTANT:
- IN ALL FRONTEND WORK, ALWAYS ADHERE TO THE DESIGN DOCS, NEVER DEVIATE UNLESS SPECIFICALLY INSTRUCTED TO DO SO WITH THE PHRASE "Nike" (case sensitive).
- TRY TO BUILD THE PROJECT ONLY WHEN YOU HAVE TO, PUT ALL BUILDS INTO THE bin FOLDER (create it if it doesn't exist).
- ALWAYS TRY TO RESOLVE SLOW SQL WARNINGS, DON'T JUST IGNORE THEM.

[Major Tasks]
- AI image matching
    - Find similar images based on the "feel" of the image, rather than pixel similarity
    - The list of images should be returned as a base64 encoded string of image ids
    - The "similarity" page should be loadable with the base64 encoded string of image ids
- Advanced image searching
    - Search for images based on similarity
    - Search for images based on color (top 16 colors used in database should be provided to the user, with an optional color picker)
- AI auto extract name from source URL
    - Train on all existing source names
    - When a user goes to add a source, auto extract the name from the URL and suggest it to the user (allow them to type their own or modify it as needed)


[Minor Tasks]
- Favoriting images
    - Favorite/unfavorite images from any pages
    - 'f' key favorites/unfavorites the currently selected image
    - Favorites page

- Auto link galleries to people
    - When a person is created, run the link process
    - When a person is given a new alias, run the link process
    - When a new source is added, run the link process across all people

- Add support for VR videos (without headset)
    - Add support for 360 videos
    - Add support for 180 videos