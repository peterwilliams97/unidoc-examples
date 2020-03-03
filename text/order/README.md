TODO
====
ChapterK.pdf p1,4,14 Need horizontal gap at bottom right
mannetal.pdf p2 top Filters out gaps between columns
             p4 bottom right. spurious gap
             p5 left. gap stops halfway down page
sunnstein.pdf p1 top middle. spurious gap
pandemic.pdf p3 spurious gap

BUGS
===

sunnstein.pdf Font widths are wrong



OLD TODO
----------
bad gap -> gap overlaps text
recognition.pdf p10 bad gap
ocr.pdf         p1,2  bad gap
invoice.pdf     p2,3 bad gap
coronaviruses.pdf p3 bad gap
Doig.pdf        p4,11,16 bad gap
20190716RES57231.pdf p6,15,16,17,18 bad gap

OLD TODO
--------
Doig.pdf         bad pivot

recognition.pdf  panic: sortX
README.pdf       Error in output PDF
sheep_dogs.pdf
bare.pdf
sunstein.pdf
ChapterK.pdf
security.orig.pdf
privacy.pdf
cloud.pdf
co2.pdf
coronaviruses.pdf
security.orig.pdf
Yamashita2018_Article_ConvolutionalNeuralNetworksAnO.pdf
20190716RES57231.pdf
Garnaut.pdf
invoice.pdf
results5.pdf
knime.pdf
mannetal.pdf
ocr.pdf


29, 42, 21, 43, 44, - 47 text only
53, 55, 59, 77 block classification

* https://www.dfki.de/fileadmin/user_upload/import/2000_HighPerfDocLayoutAna.pdf

We have developed a simple set of evaluation criteria
that identifies meaningful whitespace with an
estimated error rate of less than 0.5% on the UW3
database with a single set of parameters. The idea
is that for layout whitespace to be meaningful, it
should separate text. Therefore, we require rectangles
returned by the whitespace analysis algorithm
to be bounded by at least some minimum number
of connected components on each of its major sides.
This essentially eliminates false positive matches and
makes the algorithm nearly independent of other parameters
(such as preferred aspect ratios)


– gutters must have an aspect ratio of at least 1:3
– gutters must have a width of at least 1.5 times of the mode of the distribution of widths of inter-word spaces
– additionally, we may include prior knowledge on minimum text column widths defined by gutters
– gutters must be adjacent to at least four character-sized connected components on their left or
their right side (gutters must separate something, otherwise we are not interested in them)

Berg
====
3.3.4 Combination and filtering of column boundaries
Frequently the two introductory phases will leave us with several column boundary
candidates which effectively represent the same real boundary. While this is not critical,
it is easy to combine them. This is done by sorting the column boundary candidates
on their X–coordinate, and then combining pairs of them when there is no content
inbetween them. There is also a lower bound on column height, both because there
tended to be many falsely identified columns of short length, and because very short
columns are insignificant layout-wise since they are generally correctly grouped and
ordered anyway.


u
