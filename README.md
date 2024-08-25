![alt text](https://github.com/JaredHane98/AWS-CDK-GO-IMAGE-TRANSFORM/blob/main/Diagram.png?raw=true)


The user must provide the image transforms during the POST request

## Example
```json
{
    "ObjectName": "image.jpg",
    "Transforms": [
        {
            "Name": "grayscale"
        },
        {
            "Name": "edgedetection",
            "Params": ["0.5"]
        },
  ]
}
```
## Supported Transforms
-   grayscale
-   sharpen
-   edgedetection
-   dilate
-   erode
-   median
-   emboss
-   invert
-   sepia
-   sharpen
-   sobel


## Example Usage
```json
{
    "ObjectName": "image.jpg",
    "Transforms": [
        {
            "Name": "grayscale"
        },
        {
            "Name": "sharpen"
        },
        {
            "Name": "edgedetection",
            "Params": ["0.5"]
        },
        {
            "Name": "dilate",
            "Params": ["1"]
        },
        {
            "Name": "erode",
            "Params": ["0.75"]
        },
        {
            "Name": "median",
            "Params": ["0.24"]
        },
        {
            "Name": "emboss"
        },
        {
            "Name": "invert"
        },
        {
            "Name": "sepia"
        },
        {
            "Name": "sharpen"
        },
        {
            "Name": "sobel"
        }
    ]
}
```

## Input Image
![alt text](https://github.com/JaredHane98/AWS-CDK-GO-IMAGE-TRANSFORM/blob/main/inputimage.jpg?raw=true)

## Output Image
![alt text](https://github.com/JaredHane98/AWS-CDK-GO-IMAGE-TRANSFORM/blob/main/outputimage.jpg?raw=true)

Beaware you can easily max out memory usage of image transform and timeout the lambda. 





