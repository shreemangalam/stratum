<?xml version="1.0"?>
<xsl:stylesheet xmlns:xsl="http://www.w3.org/1999/XSL/Transform" version="1.0">
  <xsl:template match="/order">
    <invoice>
      <xsl:value-of select="total"/>
    </invoice>
  </xsl:template>
  <xsl:template name="lineItem">
    <item>
      <xsl:value-of select="price"/>
    </item>
  </xsl:template>
</xsl:stylesheet>
