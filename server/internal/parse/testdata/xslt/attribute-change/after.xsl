<?xml version="1.0"?>
<xsl:stylesheet xmlns:xsl="http://www.w3.org/1999/XSL/Transform" version="1.0">
  <xsl:template match="/payment">
    <xsl:choose>
      <xsl:when test="amount &gt; 500">
        <status>review</status>
      </xsl:when>
      <xsl:otherwise>
        <status>auto</status>
      </xsl:otherwise>
    </xsl:choose>
  </xsl:template>
</xsl:stylesheet>
